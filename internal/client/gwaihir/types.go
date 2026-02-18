package gwaihir

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// WoLSender defines the interface for sending Wake-on-LAN commands.
// This interface allows consumers to depend on the behavior rather than
// the concrete implementation, making testing and mocking easier.
type WoLSender interface {
	// SendWoL sends a Wake-on-LAN command to the specified machine.
	SendWoL(ctx context.Context, machineID string) error
}

const (
	// defaultMaxAttempts is the total number of attempts used by NewRetryConfig.
	defaultMaxAttempts = 3

	// defaultBaseDelay is the backoff base delay used by NewRetryConfig.
	defaultBaseDelay = 1 * time.Second

	// maxBackoffDelay is the ceiling applied to the exponential delay before jitter.
	maxBackoffDelay = 60 * time.Second

	// maxJitterFraction is the maximum fraction of the current delay added as random
	// jitter to avoid thundering-herd problems when multiple clients retry in concert.
	maxJitterFraction = 0.1
)

// RetryConfig holds the parameters that control retry behaviour.
//
// Use NewRetryConfig to obtain a value pre-populated with sensible defaults,
// then customise individual fields as needed.
type RetryConfig struct {
	// MaxAttempts is the total number of times the request is attempted,
	// including the initial attempt.  Must be >= 1.
	MaxAttempts int

	// BaseDelay is the delay before the second attempt.  Each subsequent
	// delay doubles (exponential back-off) up to maxBackoffDelay.  Must be > 0.
	BaseDelay time.Duration
}

// NewRetryConfig returns a RetryConfig pre-populated with sensible defaults:
// 3 total attempts and a 1 s base delay with exponential back-off capped at 60 s.
func NewRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: defaultMaxAttempts,
		BaseDelay:   defaultBaseDelay,
	}
}

// Validate returns ErrInvalidRetryConfig when the RetryConfig contains invalid values.
func (r RetryConfig) Validate() error {
	if r.MaxAttempts < 1 {
		return fmt.Errorf("%w: MaxAttempts=%d", ErrInvalidRetryConfig, r.MaxAttempts)
	}
	if r.BaseDelay <= 0 {
		return fmt.Errorf("%w: BaseDelay=%v", ErrInvalidRetryConfig, r.BaseDelay)
	}
	return nil
}

// Backoff returns the sleep duration before the nth retry (1-based).
// Backoff(1) = BaseDelay, Backoff(2) = 2×BaseDelay, capped at 60 s, plus up to 10% random jitter.
func (r RetryConfig) Backoff(retry int) time.Duration {
	delay := r.BaseDelay
	for i := 1; i < retry; i++ {
		delay *= 2
		if delay > maxBackoffDelay {
			delay = maxBackoffDelay
			break
		}
	}

	// Cap the initial delay too, in case BaseDelay itself exceeds maxBackoffDelay.
	if delay > maxBackoffDelay {
		delay = maxBackoffDelay
	}

	// Add random jitter (0–10 % of the current delay) to spread retries across
	// time and avoid thundering-herd effects.  Cryptographic randomness is not
	// required here; math/rand is intentionally used for performance.
	jitter := time.Duration(rand.Float64() * maxJitterFraction * float64(delay)) //nolint:gosec // jitter does not need crypto-grade randomness
	return delay + jitter
}

// ClientConfig defines the configuration for the Gwaihir REST client.
type ClientConfig struct {
	BaseURL     string        // Base URL of the Gwaihir service (e.g., "http://gwaihir.example.com")
	APIKey      string        // API key for authentication (sent as X-API-Key header)
	Timeout     time.Duration // HTTP request timeout (recommended: 5s)
	RetryConfig RetryConfig   // Retry configuration; use NewRetryConfig() for sensible defaults
}

// WoLRequest represents the request body for sending a Wake-on-LAN command.
type WoLRequest struct {
	MachineID string `json:"machine_id"` // Target machine ID from Gwaihir's allowlist (e.g., "saruman")
}

// WoLResponse represents the response from the Gwaihir WoL endpoint.
type WoLResponse struct {
	Message string `json:"message"` // Human-readable message (success or error description)
}
