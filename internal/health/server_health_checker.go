package health

import (
	"context"
	"errors"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

// ServerHealthChecker orchestrates health checks for a single server.
type ServerHealthChecker struct {
	serverID string            // Server identifier (matches config key)
	checker  *HealthChecker    // HTTP health checker bound to server's endpoint
	store    HealthStore       // Store for persisting health status
	logger   *logger.Logger    // Structured logger
	metrics  *metrics.Registry // Metrics registry (optional)
}

// NewServerHealthChecker creates a new ServerHealthChecker for a specific server.
//
// Parameters:
//   - serverID: Unique identifier for the server (used for logging and store keys)
//   - checker: HealthChecker already configured with the server's endpoint and timeout
//   - store: HealthStore for persisting health check results
//   - logger: Logger for structured logging
//   - m: Metrics registry (optional, can be nil)
//
// Returns a new ServerHealthChecker instance.
// Panics if any required parameter is nil/empty.
func NewServerHealthChecker(serverID string, checker *HealthChecker, store HealthStore, logger *logger.Logger, m *metrics.Registry) *ServerHealthChecker {
	if serverID == "" {
		panic("serverID cannot be empty")
	}
	if checker == nil {
		panic("checker cannot be nil")
	}
	if store == nil {
		panic("store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &ServerHealthChecker{
		serverID: serverID,
		checker:  checker,
		store:    store,
		logger:   logger,
		metrics:  m,
	}
}

// Check performs a health check for this server and updates the store with the result.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//
// Returns:
//   - ServerHealthStatus with the current health state
//   - error if the health check failed (network error, unhealthy status, etc.)
func (s *ServerHealthChecker) Check(ctx context.Context) (ServerHealthStatus, error) {
	s.logger.DebugContext(ctx, "performing health check",
		"server_id", s.serverID,
	)

	err := s.checker.CheckHealth(ctx)

	status := ServerHealthStatus{
		ServerID:      s.serverID,
		Healthy:       err == nil,
		LastCheckedAt: time.Now(),
		LastError:     "",
	}

	if err != nil {
		status.LastError = err.Error()
		s.logger.WarnContext(ctx, "health check failed",
			"server_id", s.serverID,
			"error", err,
		)
		s.recordHealthCheckFailure(err)
	} else {
		s.logger.DebugContext(ctx, "health check succeeded",
			"server_id", s.serverID,
		)
	}

	s.store.Update(s.serverID, status)

	return status, err
}

// recordHealthCheckFailure records a health check failure with the appropriate reason.
// It maps error types to metric reason labels.
func (s *ServerHealthChecker) recordHealthCheckFailure(err error) {
	if s.metrics == nil {
		return
	}

	reason := s.getFailureReason(err)
	s.metrics.Power.RecordHealthCheckFailure(s.serverID, reason)
}

// getFailureReason maps error types to metric failure reasons.
func (s *ServerHealthChecker) getFailureReason(err error) string {
	if err == nil {
		return "unknown"
	}

	// Check for context-based errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	// Check for specific health check errors (wrapped or not)
	if errors.Is(err, ErrHealthCheckNetworkError) {
		return "network_error"
	}
	if errors.Is(err, ErrHealthCheckFailed) {
		return "unhealthy_status"
	}
	if errors.Is(err, ErrHealthCheckURLMissing) {
		return "missing_url"
	}
	if errors.Is(err, ErrHealthCheckTooManyRedirects) {
		return "too_many_redirects"
	}

	return "unknown"
}
