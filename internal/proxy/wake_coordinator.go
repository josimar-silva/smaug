package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/health"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

var (
	// ErrMissingServerID is returned when WakeCoordinatorConfig.ServerID is empty.
	ErrMissingServerID = errors.New("server ID must not be empty")

	// ErrMissingMachineID is returned when WakeCoordinatorConfig.MachineID is empty.
	ErrMissingMachineID = errors.New("machine ID must not be empty")

	// ErrInvalidWakeTimeout is returned when WakeCoordinatorConfig.WakeTimeout is zero or negative.
	ErrInvalidWakeTimeout = errors.New("wake timeout must be positive")

	// ErrMissingDownstream is returned when the downstream handler is nil.
	ErrMissingDownstream = errors.New("downstream handler must not be nil")

	// ErrMissingWoLSender is returned when the WoL sender is nil.
	ErrMissingWoLSender = errors.New("WoL sender must not be nil")

	// ErrMissingHealthStore is returned when the health store is nil.
	ErrMissingHealthStore = errors.New("health store must not be nil")

	// ErrMissingLogger is returned when the logger is nil.
	ErrMissingLogger = errors.New("logger must not be nil")

	// ErrWakeTimeout is returned when the server does not become healthy within WakeTimeout.
	ErrWakeTimeout = errors.New("server did not become healthy within wake timeout")
)

const (
	// defaultPollInterval is the fallback polling interval when none is specified.
	defaultPollInterval = 500 * time.Millisecond
)

// WakeOptions holds the dependencies needed to enable Wake-on-LAN coordination in the
// RouteManager.  Pass this to RouteManager.SetWakeOptions before calling Start.
type WakeOptions struct {
	// Sender is used to send Wake-on-LAN commands via the Gwaihir service.
	Sender WoLSender
}

// WoLSender can send a Wake-on-LAN command to a named machine.
// This interface is intentionally local to the proxy package to decouple it from
// the gwaihir client implementation; gwaihir.Client satisfies it directly.
type WoLSender interface {
	SendWoL(ctx context.Context, machineID string) error
}

// WakeCoordinatorConfig contains the configuration for a WakeCoordinator.
type WakeCoordinatorConfig struct {
	ServerID     string        // Server identifier (matches config key)
	MachineID    string        // Gwaihir machine ID used in WoL requests
	WakeTimeout  time.Duration // Maximum time to wait for server to become healthy
	Debounce     time.Duration // Minimum interval between successive WoL triggers
	PollInterval time.Duration // Interval between health poll attempts (0 → defaultPollInterval)
}

// inflightWake represents a wake attempt that is currently in progress.
// Multiple goroutines can wait on done; the result is read from err after done is closed.
type inflightWake struct {
	done chan struct{} // closed when the wake attempt completes
	err  error         // result set before done is closed; safe to read after <-done
}

// WakeCoordinator is an http.Handler middleware that ensures the upstream server is
// awake before forwarding requests. When the server is unhealthy it:
//  1. Sends a single Wake-on-LAN command (debounced and coalesced across concurrent requests).
//  2. Polls the health store until the server is healthy or WakeTimeout elapses.
//     The health store is fed by the background HealthManager, so no duplicate HTTP
//     health checks are performed here.
//  3. Returns 503 Service Unavailable if the server does not recover in time.
//
// Call Close when the coordinator is no longer needed to cancel any in-progress
// wake goroutines and wait for them to exit.
type WakeCoordinator struct {
	serverID     string
	machineID    string
	wakeTimeout  time.Duration
	debounce     time.Duration
	pollInterval time.Duration

	wolSender   WoLSender
	healthStore health.HealthStore
	downstream  http.Handler
	logger      *logger.Logger

	done      chan struct{}  // closed by Close() to unblock in-flight wake goroutines
	closeOnce sync.Once      // ensures Close() is idempotent
	wg        sync.WaitGroup // tracks in-flight performWake goroutines

	mu           sync.Mutex
	lastWoLAt    time.Time
	wakeInFlight *inflightWake // non-nil while a wake attempt is in progress
}

// NewWakeCoordinator creates a new WakeCoordinator.
//
// Parameters:
//   - cfg: Coordinator configuration (server/machine IDs, timeouts, debounce).
//   - downstream: The handler to call once the server is healthy.
//   - wolSender: Used to send Wake-on-LAN commands.
//   - store: Health store polled to detect when the server becomes healthy.
//   - log: Structured logger.
//
// Possible errors:
//   - ErrMissingServerID, ErrMissingMachineID, ErrInvalidWakeTimeout
//   - ErrMissingDownstream, ErrMissingWoLSender
//   - ErrMissingHealthStore, ErrMissingLogger
func NewWakeCoordinator(
	cfg WakeCoordinatorConfig,
	downstream http.Handler,
	wolSender WoLSender,
	store health.HealthStore,
	log *logger.Logger,
) (*WakeCoordinator, error) {
	if err := validateWakeCoordinatorConfig(cfg, downstream, wolSender, store, log); err != nil {
		return nil, err
	}

	interval := cfg.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}

	return &WakeCoordinator{
		serverID:     cfg.ServerID,
		machineID:    cfg.MachineID,
		wakeTimeout:  cfg.WakeTimeout,
		debounce:     cfg.Debounce,
		pollInterval: interval,
		wolSender:    wolSender,
		healthStore:  store,
		downstream:   downstream,
		logger:       log,
		done:         make(chan struct{}),
	}, nil
}

// Close signals any in-progress wake goroutines to stop and waits for them to
// exit.  It is safe to call Close multiple times.
func (c *WakeCoordinator) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	c.wg.Wait()
	return nil
}

func validateWakeCoordinatorConfig(
	cfg WakeCoordinatorConfig,
	downstream http.Handler,
	wolSender WoLSender,
	store health.HealthStore,
	log *logger.Logger,
) error {
	if cfg.ServerID == "" {
		return ErrMissingServerID
	}
	if cfg.MachineID == "" {
		return ErrMissingMachineID
	}
	if cfg.WakeTimeout <= 0 {
		return ErrInvalidWakeTimeout
	}
	if downstream == nil {
		return ErrMissingDownstream
	}
	if wolSender == nil {
		return ErrMissingWoLSender
	}
	if store == nil {
		return ErrMissingHealthStore
	}
	if log == nil {
		return ErrMissingLogger
	}
	return nil
}

func (c *WakeCoordinator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if c.healthStore.Get(c.serverID).Healthy {
		c.downstream.ServeHTTP(w, r)
		return
	}

	c.logger.WarnContext(r.Context(), "server unhealthy, initiating wake sequence",
		"server_id", c.serverID,
		"machine_id", c.machineID,
	)

	if err := c.ensureWake(r.Context()); err != nil {
		c.logger.WarnContext(r.Context(), "wake sequence failed, returning 503",
			"server_id", c.serverID,
			"machine_id", c.machineID,
			"error", err,
		)
		http.Error(w, "Service Unavailable: server did not become healthy in time", http.StatusServiceUnavailable)
		return
	}

	c.downstream.ServeHTTP(w, r)
}

func (c *WakeCoordinator) ensureWake(ctx context.Context) error {
	c.mu.Lock()

	// Join an in-flight wake if one is already running.
	if c.wakeInFlight != nil {
		wake := c.wakeInFlight
		c.mu.Unlock()
		return c.waitForWake(ctx, wake)
	}

	// Within debounce window: don't send another WoL — just poll until healthy.
	if c.debounce > 0 && time.Since(c.lastWoLAt) < c.debounce {
		c.mu.Unlock()
		return c.pollUntilHealthy(ctx)
	}

	// Start a new wake attempt.
	wake := &inflightWake{done: make(chan struct{})}
	c.wakeInFlight = wake
	c.lastWoLAt = time.Now()
	c.mu.Unlock()

	// Run the wake asynchronously so that concurrent callers can join via waitForWake.
	// c.wg tracks this goroutine so Close() can wait for it to finish.
	c.wg.Add(1)
	go c.performWake(wake)

	return c.waitForWake(ctx, wake)
}

func (c *WakeCoordinator) waitForWake(ctx context.Context, wake *inflightWake) error {
	select {
	case <-wake.done:
		return wake.err
	case <-ctx.Done():
		return fmt.Errorf("wake interrupted: %w", ctx.Err())
	}
}

func (c *WakeCoordinator) performWake(wake *inflightWake) {
	defer c.wg.Done()

	wakeCtx, cancel := context.WithTimeout(context.Background(), c.wakeTimeout)
	defer cancel()

	c.logger.InfoContext(wakeCtx, "sending Wake-on-LAN command",
		"server_id", c.serverID,
		"machine_id", c.machineID,
	)

	if err := c.wolSender.SendWoL(wakeCtx, c.machineID); err != nil {
		c.logger.WarnContext(wakeCtx, "WoL command failed",
			"server_id", c.serverID,
			"machine_id", c.machineID,
			"error", err,
		)
	}

	err := c.pollUntilHealthyWithContext(wakeCtx)

	wake.err = err
	close(wake.done)

	c.mu.Lock()
	c.wakeInFlight = nil
	c.mu.Unlock()
}

func (c *WakeCoordinator) pollUntilHealthy(ctx context.Context) error {
	pollCtx, cancel := context.WithTimeout(ctx, c.wakeTimeout)
	defer cancel()
	return c.pollUntilHealthyWithContext(pollCtx)
}

func (c *WakeCoordinator) pollUntilHealthyWithContext(ctx context.Context) error {
	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %w", ErrWakeTimeout, ctx.Err())

		case <-c.done:
			return fmt.Errorf("%w: coordinator closed", ErrWakeTimeout)

		case <-ticker.C:
			if c.healthStore.Get(c.serverID).Healthy {
				c.logger.InfoContext(ctx, "server became healthy",
					"server_id", c.serverID,
				)
				return nil
			}
		}
	}
}
