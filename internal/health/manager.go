package health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// HealthManager coordinates health checking for all configured servers.
// It manages the lifecycle of worker goroutines that poll each server's health endpoint.
type HealthManager struct {
	config  *config.Config
	store   HealthStore
	logger  *logger.Logger
	workers []*serverWorker

	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
}

// serverWorker represents a background worker that polls health for a single server.
type serverWorker struct {
	serverID string
	interval time.Duration
	timeout  time.Duration
	checker  *ServerHealthChecker
	logger   *logger.Logger
}

// NewHealthManager creates a new HealthManager instance.
//
// Parameters:
//   - config: Application configuration containing server definitions
//   - store: HealthStore for persisting health status
//   - logger: Logger for structured logging
//
// Returns a new HealthManager instance.
// Panics if any parameter is nil.
func NewHealthManager(cfg *config.Config, store HealthStore, log *logger.Logger) *HealthManager {
	if cfg == nil {
		panic("config cannot be nil")
	}
	if store == nil {
		panic("store cannot be nil")
	}
	if log == nil {
		panic("logger cannot be nil")
	}

	return &HealthManager{
		config:  cfg,
		store:   store,
		logger:  log,
		workers: make([]*serverWorker, 0),
	}
}

// Start initializes and starts health check workers for all configured servers.
// It spawns one goroutine per server to poll health at the configured interval.
//
// Servers without a health check endpoint configured are skipped.
//
// Parameters:
//   - ctx: Context for lifecycle management. When cancelled, all workers stop.
//
// Returns error if the manager is already running.
func (m *HealthManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return fmt.Errorf("health manager is already running")
	}

	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	workerCount := 0
	for serverID, server := range m.config.Servers {
		if server.HealthCheck.Endpoint == "" {
			m.logger.Debug("skipping health check for server: no endpoint configured",
				"server_id", serverID,
			)
			continue
		}

		if server.HealthCheck.Interval <= 0 {
			m.logger.Warn("skipping health check for server: invalid interval",
				"server_id", serverID,
				"interval", server.HealthCheck.Interval,
			)
			continue
		}

		worker := newWorkerFor(server, m, serverID)

		m.workers = append(m.workers, worker)
		workerCount++

		m.wg.Add(1)
		go m.runWorker(runCtx, worker)
	}

	m.logger.Info("health check manager started",
		"worker_count", workerCount,
	)

	return nil
}

func newWorkerFor(server config.Server, m *HealthManager, serverID string) *serverWorker {
	healthChecker := NewHealthChecker(
		server.HealthCheck.Endpoint,
		server.HealthCheck.Timeout,
		m.logger,
	)

	serverChecker := NewServerHealthChecker(
		serverID,
		healthChecker,
		m.store,
		m.logger,
	)

	worker := &serverWorker{
		serverID: serverID,
		interval: server.HealthCheck.Interval,
		timeout:  server.HealthCheck.Timeout,
		checker:  serverChecker,
		logger:   m.logger,
	}

	return worker
}

// Stop gracefully stops all health check workers.
// It waits for all workers to finish with a 5-second timeout.
//
// Returns error if the manager is not running or if shutdown times out.
func (m *HealthManager) Stop() error {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return fmt.Errorf("health manager is not running")
	}

	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()

	m.logger.Info("stopping health check manager",
		"worker_count", len(m.workers),
	)

	cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("health check manager stopped successfully")
		return nil
	case <-time.After(5 * time.Second):
		m.logger.Warn("health check manager shutdown timed out")
		return fmt.Errorf("shutdown timeout: some workers did not stop in time")
	}
}

// runWorker executes the polling loop for a single server.
// This method runs in its own goroutine and exits when the context is cancelled.
func (m *HealthManager) runWorker(ctx context.Context, worker *serverWorker) {
	defer m.wg.Done()

	worker.logger.Debug("health check worker started",
		"server_id", worker.serverID,
		"interval", worker.interval,
	)

	ticker := time.NewTicker(worker.interval)
	defer ticker.Stop()

	m.performCheck(ctx, worker)

	for {
		select {
		case <-ctx.Done():
			worker.logger.Debug("health check worker stopping",
				"server_id", worker.serverID,
			)
			return
		case <-ticker.C:
			m.performCheck(ctx, worker)
		}
	}
}

// performCheck executes a single health check for a worker.
// Uses the configured health check timeout for the context deadline.
func (m *HealthManager) performCheck(ctx context.Context, worker *serverWorker) {
	checkTimeout := worker.timeout
	if checkTimeout <= 0 {
		checkTimeout = 30 * time.Second // Default fallback if not configured
	}

	checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	prevStatus := m.store.Get(worker.serverID)
	newStatus, err := worker.checker.Check(checkCtx)

	m.logStateTransition(worker.serverID, prevStatus, newStatus, err)
}

// logStateTransition logs health state changes with appropriate log levels.
func (m *HealthManager) logStateTransition(serverID string, prev, current ServerHealthStatus, err error) {
	if prev.LastCheckedAt.IsZero() {
		if current.Healthy {
			m.logger.Info("server is healthy",
				"server_id", serverID,
			)
		} else {
			m.logger.Warn("server is unhealthy",
				"server_id", serverID,
				"error", err,
			)
		}
		return
	}

	switch {
	case !prev.Healthy && current.Healthy:
		// UNHEALTHY → HEALTHY (recovery)
		m.logger.Info("server recovered",
			"server_id", serverID,
		)
	case prev.Healthy && !current.Healthy:
		// HEALTHY → UNHEALTHY (degradation)
		m.logger.Warn("server became unhealthy",
			"server_id", serverID,
			"error", err,
		)
	case current.Healthy:
		// HEALTHY → HEALTHY (no change)
		m.logger.Debug("server health check succeeded",
			"server_id", serverID,
		)
	default:
		// UNHEALTHY → UNHEALTHY (still unhealthy)
		m.logger.Debug("server still unhealthy",
			"server_id", serverID,
			"error", err,
		)
	}
}
