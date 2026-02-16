package health

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, &bytes.Buffer{})
}

// TestHealthChecker_CheckHealth_Success tests that a 200 OK response returns no error.
func TestHealthCheckerCheckHealthSuccess(t *testing.T) {
	// Given: a backend server that returns 200 OK
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	err := checker.CheckHealth(ctx)

	// Then: no error should be returned
	assert.NoError(t, err)
}

// TestHealthChecker_CheckHealth_SuccessRange tests that status codes 200-399 are considered healthy.
func TestHealthCheckerCheckHealthSuccessRange(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"202 Accepted", http.StatusAccepted},
		{"204 No Content", http.StatusNoContent},
		{"301 Moved Permanently", http.StatusMovedPermanently},
		{"302 Found", http.StatusFound},
		{"304 Not Modified", http.StatusNotModified},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a backend server that returns a success status code
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer backend.Close()

			checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
			ctx := context.Background()

			// When: checking health
			err := checker.CheckHealth(ctx)

			// Then: no error should be returned
			assert.NoError(t, err)
		})
	}
}

// TestHealthChecker_CheckHealth_Failure tests that status codes 400+ and 500+ return an error.
func TestHealthCheckerCheckHealthFailure(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: a backend server that returns an error status code
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer backend.Close()

			checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
			ctx := context.Background()

			// When: checking health
			err := checker.CheckHealth(ctx)

			// Then: an error should be returned
			assert.Error(t, err)
			assert.True(t, errors.Is(err, ErrHealthCheckFailed))
		})
	}
}

// TestHealthChecker_CheckHealth_Timeout tests that requests exceeding 2s timeout fail.
func TestHealthCheckerCheckHealthTimeout(t *testing.T) {
	// Given: a backend server that takes longer than 2s to respond
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	start := time.Now()
	err := checker.CheckHealth(ctx)
	duration := time.Since(start)

	// Then: an error should be returned and the request should timeout around 2s
	assert.Error(t, err)
	assert.Less(t, duration, 3*time.Second, "should timeout before server responds")
	assert.True(t, errors.Is(err, ErrHealthCheckNetworkError))
}

// TestHealthChecker_CheckHealth_FollowRedirects tests that up to 3 redirects are followed.
func TestHealthCheckerCheckHealthFollowRedirects(t *testing.T) {
	redirectCount := 0

	// Given: a backend server that redirects 3 times before returning 200 OK
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirectCount < 3 {
			redirectCount++
			w.Header().Set("Location", "/redirect")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	err := checker.CheckHealth(ctx)

	// Then: no error should be returned and all redirects should be followed
	assert.NoError(t, err)
	assert.Equal(t, 3, redirectCount, "should have followed 3 redirects")
}

// TestHealthChecker_CheckHealth_TooManyRedirects tests that more than 3 redirects fail.
func TestHealthCheckerCheckHealthTooManyRedirects(t *testing.T) {
	// Given: a backend server that redirects more than 3 times
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/redirect")
		w.WriteHeader(http.StatusFound)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	err := checker.CheckHealth(ctx)

	// Then: an error should be returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHealthCheckNetworkError))
}

// TestHealthChecker_CheckHealth_InvalidURL tests that invalid URLs return an error.
func TestHealthCheckerCheckHealthInvalidURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"empty URL", ""},
		{"invalid scheme", "ftp://example.com"},
		{"malformed URL", "ht!tp://invalid url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: an invalid URL
			checker := NewHealthChecker(tt.url, 2*time.Second, newTestLogger())
			ctx := context.Background()

			// When: checking health
			err := checker.CheckHealth(ctx)

			// Then: an error should be returned
			assert.Error(t, err)
		})
	}
}

// TestHealthChecker_CheckHealth_ContextCancellation tests that context cancellation stops the request.
func TestHealthCheckerCheckHealthContextCancellation(t *testing.T) {
	// Given: a backend server that takes a while to respond
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx, cancel := context.WithCancel(context.Background())

	// When: cancelling context immediately
	cancel()
	err := checker.CheckHealth(ctx)

	// Then: an error should be returned due to context cancellation
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHealthCheckNetworkError))
}

// TestHealthChecker_CheckHealth_UsesGETMethod tests that the checker uses GET method.
func TestHealthCheckerCheckHealthUsesGETMethod(t *testing.T) {
	methodUsed := ""

	// Given: a backend server that captures the HTTP method
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methodUsed = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	err := checker.CheckHealth(ctx)

	// Then: no error should be returned and GET method should be used
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, methodUsed)
}

// TestHealthChecker_CheckHealth_UnreachableServer tests behavior when server is unreachable.
func TestHealthCheckerCheckHealthUnreachableServer(t *testing.T) {
	// Given: an unreachable server address
	checker := NewHealthChecker("http://127.0.0.1:1", 2*time.Second, newTestLogger())
	ctx := context.Background()

	// When: checking health
	err := checker.CheckHealth(ctx)

	// Then: an error should be returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHealthCheckNetworkError))
}
