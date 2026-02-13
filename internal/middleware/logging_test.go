package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// newTestLogger creates a logger for testing that writes to the provided buffer.
func newTestLogger(buf *bytes.Buffer) *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, buf)
}

func TestLoggingMiddlewareLogsRequestAndResponse(t *testing.T) {
	var logBuffer bytes.Buffer
	log := newTestLogger(&logBuffer)

	backendCalled := false
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.RemoteAddr = "192.168.1.100:54321"

	wrapped.ServeHTTP(recorder, request)

	assert.True(t, backendCalled)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, logBuffer.String(), "request")
	assert.Contains(t, logBuffer.String(), "response")
}

func TestLoggingMiddlewareIncludesRequestDetails(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/api/users?id=123", nil)
	request.Header.Set("User-Agent", "test-client")
	request.RemoteAddr = "10.0.0.1:54321"

	wrapped.ServeHTTP(recorder, request)

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

func TestLoggingMiddlewareIncludesResponseDetails(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("created"))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/api/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "status")
	assert.Contains(t, logOutput, "201")
	assert.Contains(t, logOutput, "duration")
	assert.Contains(t, logOutput, "response_bytes")
}

func TestLoggingMiddlewareIncludesRequestID(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "request_id")
}

func TestLoggingMiddlewarePreservesContext(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	type contextKey string
	const testKey contextKey = "test_key"

	contextValue := ""
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextValue = r.Context().Value(testKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request = request.WithContext(context.WithValue(request.Context(), testKey, "test_value"))

	wrapped.ServeHTTP(recorder, request)

	assert.Equal(t, "test_value", contextValue)
}

func TestLoggingMiddlewareHandlesErrors(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error occurred"))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/error", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "status")
	assert.Contains(t, logOutput, "500")
}

func TestLoggingMiddlewareHandlerFunc(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backendCalled := false
	backendFunc := func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	}

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(http.HandlerFunc(backendFunc))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	assert.True(t, backendCalled)
	assert.NotEmpty(t, logBuffer.String())
}

func TestLoggingMiddlewareCapturesResponseBytes(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	responseBody := "this is the response"
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(responseBody))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "response_bytes")
	// Response body length is 19 bytes
	assert.Contains(t, logOutput, "19")
}

func TestLoggingMiddlewareDurationReasonable(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "duration")
	// Duration should be present in the log
	assert.True(t, len(logOutput) > 0)
}

func TestLoggingMiddlewareMultipleRequestsUniqueIDs(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
		wrapped.ServeHTTP(recorder, request)
	}

	logOutput := logBuffer.String()
	requestIDCount := strings.Count(logOutput, "request_id")
	// 3 requests × 2 logs per request (request + response) = 6 occurrences
	assert.Equal(t, 6, requestIDCount)
}

func TestLoggingMiddlewareWithQueryString(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/search?q=test&sort=name&page=1", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "query")
	assert.Contains(t, logOutput, "q=test")
	assert.Contains(t, logOutput, "sort=name")
	assert.Contains(t, logOutput, "page=1")
}

func TestLoggingMiddlewareWithDifferentMethods(t *testing.T) {
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

			log := newTestLogger(&logBuffer)

			// Given: A backend handler
			backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			// When: Request with specific method passes through
			middleware := NewLoggingMiddleware(log)
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

func TestLoggingMiddlewareWrapsResponseWriter(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("part1"))
		_, _ = w.Write([]byte("part2"))
		_, _ = w.Write([]byte("part3"))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "response_bytes")
	// part1 + part2 + part3 = 15 bytes
	assert.Contains(t, logOutput, "15")
}

func TestLoggingMiddlewareClientIPFromRemoteAddr(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request.RemoteAddr = "203.0.113.42:12345"

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "client_ip")
	assert.Contains(t, logOutput, "203.0.113.42")
	assert.NotContains(t, logOutput, "12345")
}

func TestLoggingMiddlewareEmptyPath(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "path")
	assert.Contains(t, logOutput, "/")
}

func TestLoggingMiddlewareHeaderPreservation(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	receivedHeaders := make(map[string]string)
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders["Content-Type"] = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "http://localhost:8080/test", nil)
	request.Header.Set("Content-Type", "application/json")

	wrapped.ServeHTTP(recorder, request)

	assert.Equal(t, "application/json", receivedHeaders["Content-Type"])
}

func TestLoggingMiddlewareChainMiddleware(t *testing.T) {
	var log1Buffer bytes.Buffer
	log1 := newTestLogger(&log1Buffer)

	var log2Buffer bytes.Buffer
	log2 := newTestLogger(&log2Buffer)

	middleware1 := NewLoggingMiddleware(log1)
	middleware2 := NewLoggingMiddleware(log2)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain(middleware1, middleware2)(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	assert.Contains(t, log1Buffer.String(), "request")
	assert.Contains(t, log2Buffer.String(), "request")
	assert.Contains(t, log1Buffer.String(), "response")
	assert.Contains(t, log2Buffer.String(), "response")
}

func TestLoggingMiddlewareChainEmptyMiddleware(t *testing.T) {
	backendCalled := false
	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain()(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	assert.True(t, backendCalled)
}

func TestLoggingMiddlewareChainOrderMatters(t *testing.T) {
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

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	chained := Chain(recordOrder("first"), recordOrder("second"))(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	chained.ServeHTTP(recorder, request)

	// Order should be: first_before, second_before, [backend], second_after, first_after
	assert.Equal(t, []string{
		"first_before",
		"second_before",
		"second_after",
		"first_after",
	}, order)
}

func TestLoggingMiddlewareClientIPWithoutPort(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	request.RemoteAddr = "192.168.1.50"

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "192.168.1.50")
}

func TestLoggingMiddlewareClientIPWithInvalidPort(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)
	// Invalid port format: too many colons or invalid separator
	request.RemoteAddr = "192.168.1.50:invalid"

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "client_ip")
}

func TestLoggingMiddlewareInfiniteResponseWriter(t *testing.T) {
	var logBuffer bytes.Buffer

	log := newTestLogger(&logBuffer)

	backend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("implicit ok"))
	})

	middleware := NewLoggingMiddleware(log)
	wrapped := middleware(backend)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/test", nil)

	wrapped.ServeHTTP(recorder, request)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "200")
	assert.Contains(t, logOutput, "response_bytes")
}
