package gwaihir

import (
	"context"
	"time"
)

// WoLSender defines the interface for sending Wake-on-LAN commands.
// This interface allows consumers to depend on the behavior rather than
// the concrete implementation, making testing and mocking easier.
type WoLSender interface {
	// SendWoL sends a Wake-on-LAN command to the specified machine.
	SendWoL(ctx context.Context, machineID string) error
}

// ClientConfig defines the configuration for the Gwaihir REST client.
type ClientConfig struct {
	BaseURL string        // Base URL of the Gwaihir service (e.g., "http://gwaihir.example.com")
	APIKey  string        // API key for authentication (sent as X-API-Key header)
	Timeout time.Duration // HTTP request timeout (recommended: 5s)
}

// WoLRequest represents the request body for sending a Wake-on-LAN command.
type WoLRequest struct {
	MachineID string `json:"machine_id"` // Target machine ID from Gwaihir's allowlist (e.g., "saruman")
}

// WoLResponse represents the response from the Gwaihir WoL endpoint.
type WoLResponse struct {
	Message string `json:"message"` // Human-readable message (success or error description)
}
