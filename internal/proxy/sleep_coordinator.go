package proxy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

var (
	// ErrSleepCoordinatorMissingTriggers is returned when NewSleepCoordinator is called
	// with a nil triggers channel.
	ErrSleepCoordinatorMissingTriggers = errors.New("sleep coordinator: triggers channel must not be nil")

	// ErrSleepCoordinatorMissingSender is returned when NewSleepCoordinator is called
	// with a nil sleep sender.
	ErrSleepCoordinatorMissingSender = errors.New("sleep coordinator: sleep sender must not be nil")

	// ErrSleepCoordinatorMissingLogger is returned when NewSleepCoordinator is called
	// with a nil logger.
	ErrSleepCoordinatorMissingLogger = errors.New("sleep coordinator: logger must not be nil")

	// ErrSleepCoordinatorAlreadyRunning is returned when Start is called on an
	// already-running coordinator.
	ErrSleepCoordinatorAlreadyRunning = errors.New("sleep coordinator: already running")

	// ErrSleepCoordinatorNotRunning is returned when Stop is called on a coordinator
	// that has not been started.
	ErrSleepCoordinatorNotRunning = errors.New("sleep coordinator: not running")
)

// SleepSender can send a sleep command to the upstream service for a given route.
// Implementations are responsible for routing the sleep command to the correct
// upstream endpoint based on the route identifier.
type SleepSender interface {
	// Sleep sends a sleep command for the given route.
	Sleep(ctx context.Context, routeID string) error
}

// SleepCoordinator listens for idle-timeout events emitted by an IdleTracker and
// calls the configured SleepSender for each triggered route.
//
// Design decisions:
//   - A single background goroutine consumes the sleep triggers channel; errors from
//     the sleep API are logged and the loop continues (no crash, no blocking).
//   - Cooldown is delegated entirely to the IdleTracker: once the tracker emits a
//     trigger it resets the route's lastSeen, so the same route cannot trigger again
//     until another full idle cycle elapses.
//   - Start/Stop follow the same pattern as IdleTracker for consistency across the
//     proxy package.
type SleepCoordinator struct {
	triggers <-chan string
	sender   SleepSender
	logger   *logger.Logger

	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{} // closed by runLoop when it exits; per-cycle
	running bool
}

// NewSleepCoordinator creates a new SleepCoordinator.
//
// Parameters:
//   - triggers: Read-only channel that delivers route IDs when they exceed their idle
//     timeout.  Typically obtained from IdleTracker.SleepTriggers().
//   - sender: Used to send sleep commands to the upstream service.
//   - log: Structured logger.
//
// Possible errors:
//   - ErrSleepCoordinatorMissingTriggers if triggers is nil.
//   - ErrSleepCoordinatorMissingSender if sender is nil.
//   - ErrSleepCoordinatorMissingLogger if log is nil.
func NewSleepCoordinator(triggers <-chan string, sender SleepSender, log *logger.Logger) (*SleepCoordinator, error) {
	if triggers == nil {
		return nil, ErrSleepCoordinatorMissingTriggers
	}

	if sender == nil {
		return nil, ErrSleepCoordinatorMissingSender
	}

	if log == nil {
		return nil, ErrSleepCoordinatorMissingLogger
	}

	return &SleepCoordinator{
		triggers: triggers,
		sender:   sender,
		logger:   log,
	}, nil
}

// Start launches the background goroutine that processes sleep triggers.
// The goroutine exits when the provided ctx is cancelled or when Stop is called.
//
// Possible errors:
//   - ErrSleepCoordinatorAlreadyRunning if Start has already been called.
func (c *SleepCoordinator) Start(ctx context.Context) error {
	c.mu.Lock()

	if c.running {
		c.mu.Unlock()
		return ErrSleepCoordinatorAlreadyRunning
	}

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.running = true

	// Pass done as a parameter so the goroutine closes the correct per-cycle channel
	// even if another Start/Stop cycle begins before this goroutine exits.
	go c.runLoop(runCtx, c.done)

	c.mu.Unlock()

	c.logger.InfoContext(ctx, "sleep coordinator started",
		"operation", "start_sleep_coordinator",
	)

	return nil
}

// Stop signals the background goroutine to exit and waits for it to finish.
//
// Possible errors:
//   - ErrSleepCoordinatorNotRunning if Start has not been called.
func (c *SleepCoordinator) Stop() error {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return ErrSleepCoordinatorNotRunning
	}

	cancel := c.cancel
	done := c.done // capture the per-cycle channel before releasing the lock
	c.running = false
	c.mu.Unlock()

	cancel()
	<-done // wait for the goroutine started by this Start/Stop cycle

	c.logger.Info("sleep coordinator stopped",
		"operation", "stop_sleep_coordinator",
	)

	return nil
}

// runLoop is the background goroutine that processes sleep triggers until the
// context is cancelled or Stop is called.
// done is the per-cycle channel closed when this goroutine exits.
func (c *SleepCoordinator) runLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("sleep coordinator loop stopping",
				"operation", "run_sleep_coordinator_loop",
			)
			return

		case routeID, ok := <-c.triggers:
			if !ok {
				// Triggers channel was closed; nothing more to process.
				c.logger.Info("sleep triggers channel closed, stopping coordinator",
					"operation", "run_sleep_coordinator_loop",
				)
				return
			}

			c.handleTrigger(ctx, routeID)
		}
	}
}

// handleTrigger calls the sleep sender for the given route.
// Errors are logged and the loop continues.
func (c *SleepCoordinator) handleTrigger(ctx context.Context, routeID string) {
	c.logger.InfoContext(ctx, "sleep trigger received, sending sleep command",
		"operation", "handle_sleep_trigger",
		"route_id", routeID,
	)

	if err := c.sender.Sleep(ctx, routeID); err != nil {
		c.logger.WarnContext(ctx, "sleep command failed",
			"operation", "handle_sleep_trigger",
			"route_id", routeID,
			"error", fmt.Errorf("%w", err),
		)
		return
	}

	c.logger.InfoContext(ctx, "sleep command sent successfully",
		"operation", "handle_sleep_trigger",
		"route_id", routeID,
	)
}
