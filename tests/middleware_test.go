//go:build integration

package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
)

// newTestLogger creates a logger for testing that writes to the provided buffer.
func newTestLogger(buf *bytes.Buffer) *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, buf)
}

// TestIntegrationLoggingMiddlewareWithProxyHandler verifies that the logging
// middleware integrates properly with a reverse proxy-like handler.
func TestIntegrationLoggingMiddlewareWithProxyHandler(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	log := newTestLogger(&logBuffer)

	// Given: A backend service
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend response"))
	}))
	defer backend.Close()

	// Given: A reverse proxy-like handler
	proxyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a reverse proxy by forwarding the request
		client := &http.Client{}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, backend.URL+r.URL.Path, r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = resp.Body.Close()
		}()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(make([]byte, resp.ContentLength))
	})

	// When: Middleware wraps the proxy handler
	mw := middleware.NewLoggingMiddleware(log)
	wrapped := mw(proxyHandler)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.RemoteAddr = "192.168.1.100:54321"

	wrapped.ServeHTTP(recorder, request)

	// Then: Both request and response are logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "request")
	assert.Contains(t, logOutput, "response")
	assert.Contains(t, logOutput, "192.168.1.100")
	assert.Contains(t, logOutput, "/api/test")
	assert.Contains(t, logOutput, "200")
}

// TestIntegrationMultipleMiddlewareChain verifies that multiple middleware
// can be chained together without interference.
func TestIntegrationMultipleMiddlewareChain(t *testing.T) {
	// Given: A logger
	var logBuffer bytes.Buffer
	log := newTestLogger(&logBuffer)

	// Given: A simple middleware that adds a header
	headerMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Processed", "true")
			next.ServeHTTP(w, r)
		})
	}

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// When: Chain logging and header middleware
	loggingMw := middleware.NewLoggingMiddleware(log)
	chained := middleware.Chain(loggingMw, middleware.Middleware(headerMiddleware))(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	// Then: Both middleware work correctly
	assert.Equal(t, "true", recorder.Header().Get("X-Processed"))
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "request")
	assert.Contains(t, logOutput, "response")
}
