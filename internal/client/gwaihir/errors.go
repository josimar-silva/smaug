package gwaihir

import "errors"

var (
	// ErrInvalidInput is returned when the input parameters are invalid (empty machine ID).
	ErrInvalidInput = errors.New("invalid input: machine ID must not be empty")

	// ErrNetworkError is returned when the HTTP request fails due to network issues.
	ErrNetworkError = errors.New("network error: failed to communicate with Gwaihir service")

	// ErrInternalError is returned when an internal error occurs (e.g., serialization failure).
	ErrInternalError = errors.New("internal error: unexpected failure in client operation")

	// ErrAuthenticationFailed is returned when the API key is invalid (401 Unauthorized).
	ErrAuthenticationFailed = errors.New("authentication failed: invalid API key")

	// ErrMachineNotFound is returned when the machine is not in Gwaihir's allowlist (404 Not Found).
	ErrMachineNotFound = errors.New("machine not found: machine is not in the allowlist")

	// ErrWoLRequestFailed is returned when the Gwaihir service returns an error response (4xx, 5xx).
	ErrWoLRequestFailed = errors.New("WoL request failed: Gwaihir service returned an error")

	// ErrEmptyBaseURL is returned when the base URL is empty during client construction.
	ErrEmptyBaseURL = errors.New("baseURL cannot be empty")

	// ErrInvalidBaseURL is returned when the base URL cannot be parsed during client construction.
	ErrInvalidBaseURL = errors.New("baseURL is not a valid URL")

	// ErrEmptyAPIKey is returned when the API key is empty during client construction.
	ErrEmptyAPIKey = errors.New("apiKey cannot be empty")

	// ErrInvalidTimeout is returned when the timeout is zero or negative during client construction.
	ErrInvalidTimeout = errors.New("timeout must be positive")

	// ErrNilLogger is returned when the logger is nil during client construction.
	ErrNilLogger = errors.New("logger cannot be nil")
)
