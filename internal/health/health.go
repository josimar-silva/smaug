package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

var (
	// ErrHealthCheckFailed is returned when a health check fails due to an unhealthy status code (4xx, 5xx).
	ErrHealthCheckFailed = errors.New("health check failed: unhealthy status code")

	// ErrHealthCheckNetworkError is returned when a health check request fails due to network issues.
	ErrHealthCheckNetworkError = errors.New("health check failed: network error")

	// ErrHealthCheckURLMissing is returned when the health check URL is not configured.
	ErrHealthCheckURLMissing = errors.New("health check URL is missing")

	// ErrHealthCheckTooManyRedirects is returned when the health check request encounters too many redirects.
	ErrHealthCheckTooManyRedirects = errors.New("health check failed: too many redirects")
)

const (
	// defaultMaxRedirects is the maximum number of redirects to follow.
	defaultMaxRedirects = 3
)

// NewHealthChecker creates a new HealthChecker for a specific endpoint.
// The checker is configured with the given timeout and follows up to 3 redirects.
//
// Parameters:
//   - endpoint: The health check endpoint URL (e.g., "http://server:8000/health")
//   - timeout: Maximum time to wait for a response (e.g., 2*time.Second)
//   - log: Structured logger for health check events
//
// Returns:
//   - A new HealthChecker instance bound to the specified endpoint
func NewHealthChecker(endpoint string, timeout time.Duration, log *logger.Logger) *HealthChecker {
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > defaultMaxRedirects {
				return ErrHealthCheckTooManyRedirects
			}
			return nil
		},
	}

	return &HealthChecker{
		endpoint: endpoint,
		timeout:  timeout,
		client:   client,
		logger:   log,
	}
}

// CheckHealth performs an HTTP GET request to the configured endpoint and returns an error
// if the response status code is not in the 200-399 range.
//
// Parameters:
//   - ctx: The context for cancellation and timeout control
//
// Returns:
//   - nil if the status code is 200-399 (healthy)
//   - ErrHealthCheckURLMissing if endpoint is empty
//   - ErrHealthCheckNetworkError (wrapped) if the request fails due to network issues
//   - ErrHealthCheckFailed (wrapped) if the status code is 400+
//
// Use errors.Is() to check for specific error types:
//
//	if errors.Is(err, ErrHealthCheckNetworkError) {
//	    // Handle network error
//	}
func (h *HealthChecker) CheckHealth(ctx context.Context) error {
	if h.endpoint == "" {
		return ErrHealthCheckURLMissing
	}

	h.logger.DebugContext(ctx, "performing health check", "url", h.endpoint)

	statusCode, err := h.executeHealthRequest(ctx)
	if err != nil {
		return err
	}

	return h.validateStatusCode(ctx, statusCode)
}

func (h *HealthChecker) executeHealthRequest(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint, nil)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to create health check request",
			"url", h.endpoint,
			"error", err,
		)
		return 0, fmt.Errorf("%w: failed to create request for %s: %v", ErrHealthCheckNetworkError, h.endpoint, err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		h.logger.WarnContext(ctx, "health check request failed",
			"url", h.endpoint,
			"error", err,
		)
		return 0, fmt.Errorf("%w: %s: %v", ErrHealthCheckNetworkError, h.endpoint, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			h.logger.WarnContext(ctx, "failed to close response body",
				"url", h.endpoint,
				"error", closeErr,
			)
		}
	}()

	return resp.StatusCode, nil
}

func (h *HealthChecker) validateStatusCode(ctx context.Context, statusCode int) error {
	if statusCode < 200 || statusCode >= 400 {
		h.logger.WarnContext(ctx, "health check failed: unhealthy status code",
			"url", h.endpoint,
			"status_code", statusCode,
		)
		return fmt.Errorf("%w: %s returned status %d", ErrHealthCheckFailed, h.endpoint, statusCode)
	}

	h.logger.DebugContext(ctx, "health check passed",
		"url", h.endpoint,
		"status_code", statusCode,
	)

	return nil
}
