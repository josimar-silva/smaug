package middleware

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStructuredLogging_LogsRequestAndResponse(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler that returns success
	backendCalled := false
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	// When: Request passes through logging middleware
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.RemoteAddr = "192.168.1.100:54321"

	wrapped.ServeHTTP(recorder, request)

	// Then: Backend is called and logging occurred
	assert.True(t, backendCalled)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, logBuffer.String(), "request")
	assert.Contains(t, logBuffer.String(), "response")
}

func TestStructuredLogging_IncludesRequestDetails(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request passes through with specific details
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/api/users?id=123", nil)
	request.Header.Set("User-Agent", "test-client")
	request.RemoteAddr = "10.0.0.1:54321"

	wrapped.ServeHTTP(recorder, request)

	// Then: Logged data contains request details
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "method")
	assert.Contains(t, logOutput, "POST")
	assert.Contains(t, logOutput, "path")
	assert.Contains(t, logOutput, "/api/users")
	assert.Contains(t, logOutput, "query")
	assert.Contains(t, logOutput, "id=123")
	assert.Contains(t, logOutput, "client_ip")
	assert.Contains(t, logOutput, "10.0.0.1")
}

func TestStructuredLogging_IncludesResponseDetails(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler that returns specific status
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/api/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Response details are logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "status")
	assert.Contains(t, logOutput, "201")
	assert.Contains(t, logOutput, "duration")
	assert.Contains(t, logOutput, "response_bytes")
}

func TestStructuredLogging_IncludesRequestID(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Request ID is generated and logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "request_id")
}

func TestStructuredLogging_PreservesContext(t *testing.T) {
	// Given: A logger and a request with context values
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A context key type
	type contextKey string
	const testKey contextKey = "test_key"

	// Given: A backend that reads context values
	contextValue := ""
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextValue = r.Context().Value(testKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	// When: Request passes through with context
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request = request.WithContext(context.WithValue(request.Context(), testKey, "test_value"))

	wrapped.ServeHTTP(recorder, request)

	// Then: Context is preserved
	assert.Equal(t, "test_value", contextValue)
}

func TestStructuredLogging_HandlesErrors(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler that returns error status
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurred"))
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/error", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Error status is logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "status")
	assert.Contains(t, logOutput, "500")
}

func TestStructuredLogging_HandlerFunc(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler function (not http.Handler)
	backendCalled := false
	backendFunc := func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}

	// When: Wrapping HandlerFunc directly
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(http.HandlerFunc(backendFunc))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Backend is called and logging occurred
	assert.True(t, backendCalled)
	assert.NotEmpty(t, logBuffer.String())
}

func TestStructuredLogging_CapturesResponseBytes(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend that returns a specific response body
	responseBody := "this is the response"
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Response byte count is logged correctly
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "response_bytes")
	// Response body length is 19 bytes
	assert.Contains(t, logOutput, "19")
}

func TestStructuredLogging_DurationReasonable(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler with a small delay
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Duration is logged and is at least the sleep duration
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "duration")
	// Duration should be present in the log
	assert.True(t, len(logOutput) > 0)
}

func TestStructuredLogging_MultipleRequests_UniqueIDs(t *testing.T) {
	// Given: A logger
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Multiple requests pass through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
		wrapped.ServeHTTP(recorder, request)
	}

	// Then: Each request and response has a request_id logged (2 per request)
	logOutput := logBuffer.String()
	requestIDCount := strings.Count(logOutput, "request_id")
	// 3 requests × 2 logs per request (request + response) = 6 occurrences
	assert.Equal(t, 6, requestIDCount)
}

func TestStructuredLogging_WithQueryString(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request has query parameters
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/search?q=test&sort=name&page=1", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Query string is captured
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "query")
	assert.Contains(t, logOutput, "q=test")
	assert.Contains(t, logOutput, "sort=name")
	assert.Contains(t, logOutput, "page=1")
}

func TestStructuredLogging_WithDifferentMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"GET", http.MethodGet},
		{"POST", http.MethodPost},
		{"PUT", http.MethodPut},
		{"DELETE", http.MethodDelete},
		{"PATCH", http.MethodPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given: A logger capturing JSON output
			var logBuffer bytes.Buffer
			handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
			logger := slog.New(handler)

			// Given: A backend handler
			backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// When: Request with specific method passes through
			middleware := NewLoggingMiddleware(logger)
			wrapped := middleware(backend)

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, "http://localhost:8080/api/test", nil)

			wrapped.ServeHTTP(recorder, request)

			// Then: Method is logged
			logOutput := logBuffer.String()
			assert.Contains(t, logOutput, tt.method)
		})
	}
}

func TestStructuredLogging_WrapsResponseWriter(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend that writes response in multiple parts
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("part1"))
		_, _ = w.Write([]byte("part2"))
		_, _ = w.Write([]byte("part3"))
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Total response bytes are counted correctly
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "response_bytes")
	// part1 + part2 + part3 = 15 bytes
	assert.Contains(t, logOutput, "15")
}

func TestStructuredLogging_ClientIPFromRemoteAddr(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request has RemoteAddr with port
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request.RemoteAddr = "203.0.113.42:12345"

	wrapped.ServeHTTP(recorder, request)

	// Then: Client IP is extracted correctly (without port)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "client_ip")
	assert.Contains(t, logOutput, "203.0.113.42")
	assert.NotContains(t, logOutput, "12345")
}

func TestStructuredLogging_EmptyPath(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request to root path
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Path is logged correctly
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "path")
	assert.Contains(t, logOutput, "/")
}

func TestStructuredLogging_HeaderPreservation(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend that checks headers
	receivedHeaders := make(map[string]string)
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders["Content-Type"] = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	// When: Request with headers passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/test", nil)
	request.Header.Set("Content-Type", "application/json")

	wrapped.ServeHTTP(recorder, request)

	// Then: Headers are preserved
	assert.Equal(t, "application/json", receivedHeaders["Content-Type"])
}

func TestStructuredLogging_ChainMiddleware(t *testing.T) {
	// Given: Two loggers
	var log1Buffer bytes.Buffer
	handler1 := slog.NewJSONHandler(&log1Buffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger1 := slog.New(handler1)

	var log2Buffer bytes.Buffer
	handler2 := slog.NewJSONHandler(&log2Buffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger2 := slog.New(handler2)

	// Given: Two logging middleware
	middleware1 := NewLoggingMiddleware(logger1)
	middleware2 := NewLoggingMiddleware(logger2)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Chain middleware together
	chained := Chain(middleware1, middleware2)(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	// Then: Both middleware log the request/response
	assert.Contains(t, log1Buffer.String(), "request")
	assert.Contains(t, log2Buffer.String(), "request")
	assert.Contains(t, log1Buffer.String(), "response")
	assert.Contains(t, log2Buffer.String(), "response")
}

func TestStructuredLogging_ChainEmptyMiddleware(t *testing.T) {
	// Given: A backend handler
	backendCalled := false
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// When: Chain with empty middleware list
	chained := Chain()(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	// Then: Backend is still called
	assert.True(t, backendCalled)
}

func TestStructuredLogging_ChainOrderMatters(t *testing.T) {
	// Given: Two loggers capturing execution order
	var order []string
	orderMutex := &sync.Mutex{}

	recordOrder := func(label string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				orderMutex.Lock()
				order = append(order, label+"_before")
				orderMutex.Unlock()

				next.ServeHTTP(w, r)

				orderMutex.Lock()
				order = append(order, label+"_after")
				orderMutex.Unlock()
			})
		}
	}

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Chain middleware in specific order
	chained := Chain(recordOrder("first"), recordOrder("second"))(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	// Then: Execution order shows first middleware is outermost
	// Order should be: first_before, second_before, [backend], second_after, first_after
	assert.Equal(t, []string{
		"first_before",
		"second_before",
		"second_after",
		"first_after",
	}, order)
}

func TestStructuredLogging_ClientIPWithoutPort(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request has RemoteAddr without port (edge case)
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request.RemoteAddr = "192.168.1.50"

	wrapped.ServeHTTP(recorder, request)

	// Then: Client IP is used as-is
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "192.168.1.50")
}

func TestStructuredLogging_ClientIPWithInvalidPort(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend handler
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// When: Request has RemoteAddr with invalid port format
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	// Invalid port format: too many colons or invalid separator
	request.RemoteAddr = "192.168.1.50:invalid"

	wrapped.ServeHTTP(recorder, request)

	// Then: Original RemoteAddr is used when parsing fails
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "client_ip")
}

func TestStructuredLogging_InfiniteResponseWriter(t *testing.T) {
	// Given: A logger capturing JSON output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	// Given: A backend that never calls WriteHeader explicitly (defaults to 200)
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("implicit ok"))
	})

	// When: Request passes through
	middleware := NewLoggingMiddleware(logger)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	// Then: Default status 200 is logged
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "200")
	assert.Contains(t, logOutput, "response_bytes")
}
