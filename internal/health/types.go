package health

import (
	"net/http"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// ServerHealthStatus represents the health state of a server at a point in time.
type ServerHealthStatus struct {
	ServerID      string    // Server identifier (matches config key)
	Healthy       bool      // Current health state (true=healthy, false=unhealthy/unknown)
	LastCheckedAt time.Time // Timestamp of last health check
	LastError     string    // Error message from last failed check (empty if healthy)
}

// HealthChecker performs HTTP health checks on a specific backend endpoint.
// Each HealthChecker is bound to a single endpoint with its own timeout configuration.
type HealthChecker struct {
	endpoint string         // The health check endpoint URL
	timeout  time.Duration  // Maximum time to wait for a response
	client   *http.Client   // HTTP client for making requests
	logger   *logger.Logger // Structured logger
}
