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

// HealthStore provides thread-safe access to server health status.
type HealthStore interface {
	// Get retrieves the current health status for a server.
	// Returns (status, true) if the server has been health-checked, (zero-value, false) otherwise.
	Get(serverID string) (ServerHealthStatus, bool)

	// Update stores a new health status for a server.
	// This operation is atomic and thread-safe.
	Update(serverID string, status ServerHealthStatus)

	// GetAll returns health status for all servers.
	// Returns a copy of the internal state to prevent external mutation.
	GetAll() map[string]ServerHealthStatus
}

// HealthChecker performs HTTP health checks on a specific backend endpoint.
// Each HealthChecker is bound to a single endpoint with its own timeout configuration.
type HealthChecker struct {
	endpoint  string         // The health check endpoint URL
	timeout   time.Duration  // Maximum time to wait for a response
	authToken string         // Optional base64 user:password token for Basic Auth
	client    *http.Client   // HTTP client for making requests
	logger    *logger.Logger // Structured logger
}
