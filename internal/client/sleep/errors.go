package sleep

import "errors"

var (
	// ErrEmptyEndpoint is returned when the endpoint URL is empty during client construction.
	ErrEmptyEndpoint = errors.New("endpoint cannot be empty")

	// ErrInvalidEndpoint is returned when the endpoint URL has an unsupported scheme.
	// Only http and https are allowed.
	ErrInvalidEndpoint = errors.New("endpoint must use http or https scheme")

	// ErrSSRFAttempt is returned when the endpoint resolves to a loopback or localhost
	// address, which would constitute a Server-Side Request Forgery (SSRF) risk.
	ErrSSRFAttempt = errors.New("endpoint targets a loopback or localhost address: SSRF not allowed")

	// ErrInvalidTimeout is returned when the timeout is zero or negative during client construction.
	ErrInvalidTimeout = errors.New("timeout must be positive")

	// ErrNilLogger is returned when the logger is nil during client construction.
	ErrNilLogger = errors.New("logger cannot be nil")

	// ErrNetworkError is returned when the HTTP request fails due to network issues.
	ErrNetworkError = errors.New("network error: failed to communicate with sleep endpoint")

	// ErrSleepRequestFailed is returned when the sleep endpoint returns an error response.
	ErrSleepRequestFailed = errors.New("sleep request failed: endpoint returned an error")
)
