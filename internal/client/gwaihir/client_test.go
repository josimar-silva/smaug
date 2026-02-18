package gwaihir

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// testServerWithErrorResponse creates a test server that returns an error response.
func testServerWithErrorResponse(statusCode int, message string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		resp := WoLResponse{
			Message: message,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// TestNewClientValidParameters tests successful client construction.
func TestNewClientValidParameters(t *testing.T) {
	// Given: valid parameters
	config := ClientConfig{
		BaseURL:     "http://gwaihir.example.com",
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	log := newTestLogger()

	// When: creating a Gwaihir client
	client, err := NewClient(config, log)

	// Then: should create successfully
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, config.BaseURL, client.baseURL)
	assert.Equal(t, config.APIKey, client.apiKey)
	assert.Equal(t, config.Timeout, client.timeout)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.logger)
}

// TestNewClientInvalidParameters tests constructor validation.
func TestNewClientInvalidParameters(t *testing.T) {
	validConfig := ClientConfig{
		BaseURL:     "http://gwaihir.example.com",
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	log := newTestLogger()

	testCases := []struct {
		name        string
		config      ClientConfig
		logger      *logger.Logger
		expectedErr error
	}{
		{
			name: "empty baseURL",
			config: ClientConfig{
				BaseURL:     "",
				APIKey:      "test-api-key",
				Timeout:     5 * time.Second,
				RetryConfig: NewRetryConfig(),
			},
			logger:      log,
			expectedErr: ErrEmptyBaseURL,
		},
		{
			name: "empty apiKey",
			config: ClientConfig{
				BaseURL:     "http://gwaihir.example.com",
				APIKey:      "",
				Timeout:     5 * time.Second,
				RetryConfig: NewRetryConfig(),
			},
			logger:      log,
			expectedErr: ErrEmptyAPIKey,
		},
		{
			name: "zero timeout",
			config: ClientConfig{
				BaseURL:     "http://gwaihir.example.com",
				APIKey:      "test-api-key",
				Timeout:     0,
				RetryConfig: NewRetryConfig(),
			},
			logger:      log,
			expectedErr: ErrInvalidTimeout,
		},
		{
			name: "negative timeout",
			config: ClientConfig{
				BaseURL:     "http://gwaihir.example.com",
				APIKey:      "test-api-key",
				Timeout:     -5 * time.Second,
				RetryConfig: NewRetryConfig(),
			},
			logger:      log,
			expectedErr: ErrInvalidTimeout,
		},
		{
			name:        "nil logger",
			config:      validConfig,
			logger:      nil,
			expectedErr: ErrNilLogger,
		},
		{
			name:        "valid parameters",
			config:      validConfig,
			logger:      log,
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// When: creating a client with the given parameters
			client, err := NewClient(tc.config, tc.logger)

			// Then: should return the expected error
			if tc.expectedErr != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedErr), "expected error %v, got %v", tc.expectedErr, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

// TestClientSendWoLSuccess tests successful WoL command sending.
func TestClientSendWoLSuccess(t *testing.T) {
	// Given: a mock Gwaihir server that returns success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HTTP method
		assert.Equal(t, http.MethodPost, r.Method)

		// Verify path
		assert.Equal(t, "/wol", r.URL.Path)

		// Verify X-API-Key header
		assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))

		// Verify Content-Type header
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var req WoLRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)
		assert.Equal(t, "saruman", req.MachineID)

		// Return success response (202 Accepted as per Gwaihir API)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		resp := WoLResponse{
			Message: "WoL packet sent successfully",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should succeed
	assert.NoError(t, err)
}

// TestClientSendWoLServerError tests handling of server errors.
func TestClientSendWoLServerError(t *testing.T) {
	// Given: a mock Gwaihir server that returns an error
	server := testServerWithErrorResponse(http.StatusInternalServerError, "Internal server error")
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should fail with appropriate error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Contains(t, err.Error(), "500")
}

// testServerWithErrorResponse tests handling of authentication errors.
func TestClientSendWoLAuthenticationError(t *testing.T) {
	// Given: a mock Gwaihir server that returns 401 Unauthorized
	server := testServerWithErrorResponse(http.StatusUnauthorized, "Invalid API key")
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "invalid-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should fail with authentication error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthenticationFailed))
	assert.Contains(t, err.Error(), "401")
}

// TestClientSendWoLNetworkError tests handling of network errors.
func TestClientSendWoLNetworkError(t *testing.T) {
	// Given: an unreachable Gwaihir server
	config := ClientConfig{
		BaseURL:     "http://192.0.2.1:9999", // TEST-NET-1 (unreachable)
		APIKey:      "test-api-key",
		Timeout:     100 * time.Millisecond,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should fail with network error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNetworkError))
}

// TestClientSendWoLInvalidURLInRequest tests that NewClient rejects an invalid base URL at
// construction time rather than allowing a broken client to be used later.
func TestClientSendWoLInvalidURLInRequest(t *testing.T) {
	// Given: a configuration with an invalid base URL
	config := ClientConfig{
		BaseURL:     "ht tp://invalid url with spaces", // Invalid URL format
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}

	// When: creating a Gwaihir client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail at construction with ErrInvalidBaseURL so the bad URL is
	// caught early instead of causing an obscure network error at request time.
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidBaseURL))
	assert.Nil(t, client)
}

// TestClientSendWoLTimeoutError tests handling of timeout errors.
func TestClientSendWoLTimeoutError(t *testing.T) {
	// Given: a slow Gwaihir server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     50 * time.Millisecond, // Shorter than server response time
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should fail with timeout error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNetworkError))
}

// TestClientSendWoLContextCancellation tests handling of context cancellation.
// When the context is already cancelled before the first attempt, the HTTP call
// returns a network-level error.  With MaxAttempts > 1 the retry backoff also
// detects the cancelled context and short-circuits, so the error propagated to
// the caller is a context error (not necessarily ErrNetworkError).
func TestClientSendWoLContextCancellation(t *testing.T) {
	// Given: a slow Gwaihir server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err = client.SendWoL(ctx, "saruman")

	// Then: should fail with any error (context or network-level)
	assert.Error(t, err)
}

// TestClientSendWoLInvalidMachineID tests validation of machine ID.
func TestClientSendWoLInvalidMachineID(t *testing.T) {
	// Given: a valid Gwaihir client
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command with empty machine ID
	ctx := context.Background()
	err = client.SendWoL(ctx, "")

	// Then: should fail with validation error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidInput))
}

// TestClientSendWoLMachineNotFound tests handling of 404 Not Found (machine not in allowlist).
func TestClientSendWoLMachineNotFound(t *testing.T) {
	// Given: a mock Gwaihir server that returns 404
	server := testServerWithErrorResponse(http.StatusNotFound, "Machine not found in allowlist")
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command for a non-existent machine
	ctx := context.Background()
	err = client.SendWoL(ctx, "gandalf")

	// Then: should fail with machine not found error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMachineNotFound))
	assert.Contains(t, err.Error(), "404")
}

// TestClientSendWoLErrorResponseParsing tests parsing of error responses.
func TestClientSendWoLErrorResponseParsing(t *testing.T) {
	testCases := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedError  error
		errorSubstring string
	}{
		{
			name:       "400 Bad Request",
			statusCode: http.StatusBadRequest,
			responseBody: `{
				"message": "Invalid request body"
			}`,
			expectedError:  ErrWoLRequestFailed,
			errorSubstring: "Invalid request body",
		},
		{
			name:       "401 Unauthorized",
			statusCode: http.StatusUnauthorized,
			responseBody: `{
				"message": "Invalid API key"
			}`,
			expectedError:  ErrAuthenticationFailed,
			errorSubstring: "Invalid API key",
		},
		{
			name:       "404 Not Found",
			statusCode: http.StatusNotFound,
			responseBody: `{
				"message": "Machine not found in allowlist"
			}`,
			expectedError:  ErrMachineNotFound,
			errorSubstring: "Machine not found in allowlist",
		},
		{
			name:       "500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
			responseBody: `{
				"message": "Failed to send WoL packet"
			}`,
			expectedError:  ErrWoLRequestFailed,
			errorSubstring: "Failed to send WoL packet",
		},
		{
			name:           "Malformed JSON response",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{invalid json`,
			expectedError:  ErrWoLRequestFailed,
			errorSubstring: "500",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a mock Gwaihir server that returns the specified error
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			config := ClientConfig{
				BaseURL:     server.URL,
				APIKey:      "test-api-key",
				Timeout:     5 * time.Second,
				RetryConfig: NewRetryConfig(),
			}
			client, err := NewClient(config, newTestLogger())
			assert.NoError(t, err)

			// When: sending a WoL command
			ctx := context.Background()
			err = client.SendWoL(ctx, "saruman")

			// Then: should fail with expected error
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tc.expectedError))
			assert.Contains(t, err.Error(), tc.errorSubstring)
		})
	}
}

// TestClientSendWoLSuccessWithMalformedJSON tests handling of malformed JSON in success response.
func TestClientSendWoLSuccessWithMalformedJSON(t *testing.T) {
	// Given: a mock Gwaihir server that returns success with malformed JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted) // 202 Accepted as per Gwaihir API
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should still succeed (status code 2xx is what matters)
	assert.NoError(t, err)
}

// TestClientSendWoLSuccessResponseVariations tests different success response formats.
func TestClientSendWoLSuccessResponseVariations(t *testing.T) {
	testCases := []struct {
		name         string
		responseBody string
	}{
		{
			name: "success with message",
			responseBody: `{
				"message": "WoL packet sent successfully"
			}`,
		},
		{
			name: "success with empty message",
			responseBody: `{
				"message": ""
			}`,
		},
		{
			name: "success with additional fields (ignored)",
			responseBody: `{
				"message": "WoL packet sent",
				"timestamp": "2024-02-16T12:00:00Z"
			}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a mock Gwaihir server that returns success
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted) // 202 Accepted as per Gwaihir API
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer server.Close()

			config := ClientConfig{
				BaseURL:     server.URL,
				APIKey:      "test-api-key",
				Timeout:     5 * time.Second,
				RetryConfig: NewRetryConfig(),
			}
			client, err := NewClient(config, newTestLogger())
			assert.NoError(t, err)

			// When: sending a WoL command
			ctx := context.Background()
			err = client.SendWoL(ctx, "saruman")

			// Then: should succeed
			assert.NoError(t, err)
		})
	}
}

// TestClientSendWoLErrorWithMalformedJSON tests error responses that carry malformed JSON bodies.
func TestClientSendWoLErrorWithMalformedJSON(t *testing.T) {
	testCases := []struct {
		name        string
		statusCode  int
		apiKey      string
		machineID   string
		expectedErr error
		errSubstr   string
	}{
		{
			name:        "401 with malformed JSON",
			statusCode:  http.StatusUnauthorized,
			apiKey:      "invalid-key",
			machineID:   "saruman",
			expectedErr: ErrAuthenticationFailed,
			errSubstr:   "401",
		},
		{
			name:        "404 with malformed JSON",
			statusCode:  http.StatusNotFound,
			apiKey:      "test-api-key",
			machineID:   "unknown-machine",
			expectedErr: ErrMachineNotFound,
			errSubstr:   "404",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Given: a server that returns the given status with malformed JSON
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(`{invalid json`))
			}))
			defer server.Close()

			config := ClientConfig{
				BaseURL:     server.URL,
				APIKey:      tc.apiKey,
				Timeout:     5 * time.Second,
				RetryConfig: NewRetryConfig(),
			}
			client, err := NewClient(config, newTestLogger())
			assert.NoError(t, err)

			// When: sending a WoL command
			err = client.SendWoL(context.Background(), tc.machineID)

			// Then: the domain error is returned even without a parseable body
			assert.Error(t, err)
			assert.True(t, errors.Is(err, tc.expectedErr))
			assert.Contains(t, err.Error(), tc.errSubstr)
		})
	}
}

// TestClientSendWoLLargeResponseBody tests handling of large response bodies.
func TestClientSendWoLLargeResponseBody(t *testing.T) {
	// Given: a mock Gwaihir server that returns a large response
	largeBody := make([]byte, 2*1024*1024) // 2 MB, exceeds 1 MB limit
	for i := range largeBody {
		largeBody[i] = 'A'
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(largeBody)
	}))
	defer server.Close()

	config := ClientConfig{
		BaseURL:     server.URL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	ctx := context.Background()
	err = client.SendWoL(ctx, "saruman")

	// Then: should handle the large response (truncated to 1MB)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
}
