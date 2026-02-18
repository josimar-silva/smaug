package sleep

import (
	"context"
	"time"
)

// defaultTimeout is the HTTP request timeout used by NewClientConfig.
const defaultTimeout = 10 * time.Second

// Sleeper defines the interface for sending sleep commands to a remote endpoint.
// This interface allows consumers to depend on the behaviour rather than the
// concrete implementation, making testing and mocking easier.
type Sleeper interface {
	// Sleep sends a sleep command to the configured endpoint.
	Sleep(ctx context.Context) error
}

// ClientConfig defines the configuration for the sleep REST client.
type ClientConfig struct {
	Endpoint string        // Full URL of the sleep endpoint (e.g., "http://homeserver.local/sleep")
	Timeout  time.Duration // HTTP request timeout; use defaultTimeout (10s) as a sensible value
}

// NewClientConfig returns a ClientConfig pre-populated with a sensible default timeout (10 s).
func NewClientConfig(endpoint string) ClientConfig {
	return ClientConfig{
		Endpoint: endpoint,
		Timeout:  defaultTimeout,
	}
}
