package proxy

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

var (
	// ErrIdleTrackerMissingLogger is returned when NewIdleTracker is called with a nil logger.
	ErrIdleTrackerMissingLogger = errors.New("idle tracker: logger must not be nil")

	// ErrIdleTrackerInvalidCheckInterval is returned when NewIdleTracker is called with a
	// zero or negative check interval.
	ErrIdleTrackerInvalidCheckInterval = errors.New("idle tracker: check interval must be positive")

	// ErrIdleTrackerAlreadyRunning is returned when Start is called on an already-running tracker.
	ErrIdleTrackerAlreadyRunning = errors.New("idle tracker: already running")

	// ErrIdleTrackerNotRunning is returned when Stop is called on a tracker that is not running.
	ErrIdleTrackerNotRunning = errors.New("idle tracker: not running")
)

// idleNeverSeen is the sentinel duration returned by IdleTime when no activity has ever
// been recorded for a route.  It is intentionally large so that callers treat the route
// as always having been idle.
const idleNeverSeen = time.Duration(1<<63 - 1) // math.MaxInt64

// sleepTriggerBufSize is the capacity of the sleep trigger channel.  A buffered channel
// avoids blocking the background goroutine when the consumer is slow.
const sleepTriggerBufSize = 64

// IdleTrackerConfig holds the configuration for an IdleTracker.
type IdleTrackerConfig struct {
	// CheckInterval is the period between background idle checks.
	// Must be positive.  The production default is 1 minute.
	CheckInterval time.Duration
}

// routeEntry stores per-route state used by the idle tracker.
type routeEntry struct {
	idleTimeout time.Duration
	lastSeen    time.Time
}

// IdleTracker tracks the last activity time for each registered route and emits
// sleep-trigger signals on a channel when a route exceeds its idle timeout.
//
// Design decisions:
//   - A single background goroutine fires at CheckInterval; it acquires a write
//     lock over all route entries to both inspect idle durations and reset
//     lastSeen on triggered routes atomically.
//   - Activity recording is write-locked only for the affected route's timestamp.
//   - The sleep trigger channel is buffered to decouple the background goroutine
//     from slow consumers.
//   - Once a sleep trigger is successfully delivered for a route, the route's
//     lastSeen is reset to now so that subsequent checks don't immediately
//     re-trigger.  Dropped triggers (channel full) are retried on every tick
//     until the channel drains.
//   - Each Start/Stop cycle uses a dedicated done channel so that a concurrent
//     Start cannot cause Stop to wait on the wrong goroutine.
type IdleTracker struct {
	cfg     IdleTrackerConfig
	logger  *logger.Logger
	metrics *metrics.Registry // Metrics registry (optional)

	mu     sync.RWMutex
	routes map[string]*routeEntry

	sleepCh chan string
	cancel  context.CancelFunc
	done    chan struct{} // closed by runChecker when it exits; per-cycle
	running bool
}

// NewIdleTracker creates a new IdleTracker with the given configuration and logger.
//
// Possible errors:
//   - ErrIdleTrackerMissingLogger if log is nil.
//   - ErrIdleTrackerInvalidCheckInterval if cfg.CheckInterval is zero or negative.
func NewIdleTracker(cfg IdleTrackerConfig, log *logger.Logger) (*IdleTracker, error) {
	if log == nil {
		return nil, ErrIdleTrackerMissingLogger
	}
	if cfg.CheckInterval <= 0 {
		return nil, ErrIdleTrackerInvalidCheckInterval
	}

	return &IdleTracker{
		cfg:     cfg,
		logger:  log,
		routes:  make(map[string]*routeEntry),
		sleepCh: make(chan string, sleepTriggerBufSize),
	}, nil
}

// SetMetrics sets the metrics registry for recording idle sleep triggers.
// Must be called before Start.
func (t *IdleTracker) SetMetrics(reg *metrics.Registry) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		t.logger.Warn("SetMetrics called after Start; ignoring")
		return
	}

	t.metrics = reg
}

// RegisterRoute registers a route with the tracker and associates it with the given
// idle timeout.  Calling RegisterRoute for an existing route updates its idle timeout
// but does NOT reset its last-seen time.
func (t *IdleTracker) RegisterRoute(routeID string, idleTimeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if existing, ok := t.routes[routeID]; ok {
		existing.idleTimeout = idleTimeout
		return
	}

	t.routes[routeID] = &routeEntry{
		idleTimeout: idleTimeout,
	}
}

// RecordActivity records that the given route has seen activity at the current time.
// If the route has not been registered yet, RecordActivity registers it with a zero
// idle timeout (the route will never trigger a sleep via the background checker).
func (t *IdleTracker) RecordActivity(routeID string) {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	entry, ok := t.routes[routeID]
	if !ok {
		// Auto-register with zero timeout so we track activity even without explicit registration.
		entry = &routeEntry{}
		t.routes[routeID] = entry
	}
	entry.lastSeen = now
}

// IdleTime returns how long ago activity was last recorded for the given route.
// If no activity has ever been recorded, it returns idleNeverSeen (math.MaxInt64),
// treating the route as having been idle forever.
func (t *IdleTracker) IdleTime(routeID string) time.Duration {
	t.mu.RLock()
	entry, ok := t.routes[routeID]
	var lastSeen time.Time
	if ok {
		lastSeen = entry.lastSeen // copy while the lock is held to avoid a data race
	}
	t.mu.RUnlock()

	if !ok {
		return idleNeverSeen
	}

	return computeIdleTime(lastSeen, time.Now())
}

// SleepTriggers returns the channel on which route IDs are sent when their idle
// timeout is exceeded.  The same channel is returned on every call.
func (t *IdleTracker) SleepTriggers() <-chan string {
	return t.sleepCh
}

// Start launches the background goroutine that checks for idle routes at the
// configured interval.  The goroutine exits when the provided ctx is cancelled or
// when Stop is called.
//
// Possible errors:
//   - ErrIdleTrackerAlreadyRunning if Start has already been called.
func (t *IdleTracker) Start(ctx context.Context) error {
	t.mu.Lock()

	if t.running {
		t.mu.Unlock()
		return ErrIdleTrackerAlreadyRunning
	}

	runCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	t.done = make(chan struct{})
	t.running = true

	// Pass done as a parameter: it is evaluated now, while the lock is held and
	// t.done is guaranteed to belong to this cycle.  A deferred close(t.done)
	// inside the goroutine would read the struct field at return-time and could
	// close a future cycle's channel if Start is called again before G exits.
	go t.runChecker(runCtx, t.done)

	// Release the lock before logging so RecordActivity/RegisterRoute callers on
	// the hot request path are not blocked by the log write.
	t.mu.Unlock()

	t.logger.InfoContext(ctx, "idle tracker started",
		"operation", "start_idle_tracker",
		"check_interval", t.cfg.CheckInterval,
	)

	return nil
}

// Stop signals the background goroutine to exit and waits for it to finish.
// Returns ErrIdleTrackerNotRunning if Start has not been called or the tracker
// has already been stopped.
//
// Possible errors:
//   - ErrIdleTrackerNotRunning if Start has not been called.
func (t *IdleTracker) Stop() error {
	t.mu.Lock()

	if !t.running {
		t.mu.Unlock()
		return ErrIdleTrackerNotRunning
	}

	cancel := t.cancel
	done := t.done // capture the per-cycle channel before releasing the lock
	t.running = false
	t.mu.Unlock()

	cancel()
	<-done // wait only for the goroutine started by this Start/Stop cycle

	t.logger.Info("idle tracker stopped", "operation", "stop_idle_tracker")

	return nil
}

// runChecker is the background goroutine that periodically checks for idle routes.
// done is the per-cycle channel that is closed when this goroutine exits.
func (t *IdleTracker) runChecker(ctx context.Context, done chan struct{}) {
	defer close(done)

	ticker := time.NewTicker(t.cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// ctx is already cancelled here; use the non-context logger so the
			// message is always emitted regardless of the logger's context handling.
			t.logger.Info("idle tracker checker stopping", "operation", "run_idle_checker")
			return

		case <-ticker.C:
			t.checkIdleRoutes(ctx)
		}
	}
}

// checkIdleRoutes inspects all registered routes and emits a sleep trigger for any
// that have exceeded their idle timeout.
func (t *IdleTracker) checkIdleRoutes(ctx context.Context) {
	now := time.Now()

	t.mu.Lock()
	defer t.mu.Unlock()

	for routeID, entry := range t.routes {
		if entry.idleTimeout <= 0 {
			// Route has no idle timeout configured; skip.
			continue
		}

		idleSince := computeIdleTime(entry.lastSeen, now)
		if idleSince < entry.idleTimeout {
			continue
		}

		t.logger.InfoContext(ctx, "route idle timeout exceeded, emitting sleep trigger",
			"operation", "check_idle_routes",
			"route_id", routeID,
			"idle_duration", idleSince,
			"idle_timeout", entry.idleTimeout,
		)

		// Non-blocking send.  lastSeen is reset only on a successful send so that
		// a dropped trigger (channel full) is retried on every subsequent tick
		// until the consumer drains the channel.
		select {
		case t.sleepCh <- routeID:
			// Reset lastSeen to now so the route must idle for another full
			// idleTimeout before the next trigger is emitted.
			entry.lastSeen = now
			// Record sleep trigger metric
			if t.metrics != nil {
				t.metrics.Power.RecordSleepTriggered(routeID)
			}
		default:
			t.logger.WarnContext(ctx, "sleep trigger channel full, dropping trigger",
				"operation", "check_idle_routes",
				"route_id", routeID,
			)
		}
	}
}

// computeIdleTime returns how long the route has been idle.
// If lastSeen is the zero value, returns idleNeverSeen.
func computeIdleTime(lastSeen, now time.Time) time.Duration {
	if lastSeen.IsZero() {
		return idleNeverSeen
	}
	return now.Sub(lastSeen)
}
