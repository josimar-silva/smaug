package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// mockActivityRecorder is a test double for ActivityRecorder that records all calls.
type mockActivityRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockActivityRecorder) RecordActivity(routeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, routeID)
}

func (m *mockActivityRecorder) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockActivityRecorder) lastRouteID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return ""
	}
	return m.calls[len(m.calls)-1]
}

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, &bytes.Buffer{})
}

func TestProxyHandlerSuccess(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend response"))
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "backend response", recorder.Body.String())
}

func TestProxyHandlerPreservesHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwardedFor := r.Header.Get(HeaderXForwardedFor)
		forwardedProto := r.Header.Get(HeaderXForwardedProto)
		host := r.Header.Get(HeaderXForwardedHost)

		if forwardedFor != "" && forwardedProto != "" && host != "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("headers-ok"))
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.Header.Set("User-Agent", "test-agent")
	request.RemoteAddr = "192.168.1.100:12345"

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "headers-ok", recorder.Body.String())
}

func TestProxyHandlerPreservesUserAgent(t *testing.T) {
	userAgentSeen := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgentSeen = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.Header.Set("User-Agent", "custom-agent/1.0")

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "custom-agent/1.0", userAgentSeen)
}

func TestProxyHandlerBackendUnreachableReturns502(t *testing.T) {
	proxy := NewProxyHandler("http://127.0.0.1:1", newTestLogger())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestProxyHandlerPreservesRequestPath(t *testing.T) {
	pathSeen := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathSeen = r.RequestURI
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/v1/users?id=123", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, pathSeen, "/api/v1/users")
	assert.Contains(t, pathSeen, "id=123")
}

func TestProxyHandlerBackendErrorPropagatesToClient(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("backend error"))
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "backend error", recorder.Body.String())
}

func TestProxyHandlerDifferentMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
	}{
		{"GET", http.MethodGet},
		{"POST", http.MethodPost},
		{"PUT", http.MethodPut},
		{"DELETE", http.MethodDelete},
		{"PATCH", http.MethodPatch},
		{"HEAD", http.MethodHead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			methodSeen := ""
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methodSeen = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			proxy := NewProxyHandler(backend.URL, newTestLogger())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, "http://localhost:8080/api/test", nil)

			proxy.ServeHTTP(recorder, request)

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, tt.method, methodSeen)
		})
	}
}

func TestProxyHandlerResponseHeaders(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "custom-value", recorder.Header().Get("X-Custom-Header"))
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.Equal(t, `{"result":"ok"}`, recorder.Body.String())
}

func TestProxyHandlerLargeResponse(t *testing.T) {
	largeBody := make([]byte, 1024*1024) // 1MB
	for i := range largeBody {
		largeBody[i] = byte(i % 256)
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(largeBody)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/large", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, len(largeBody), len(recorder.Body.Bytes()))
}

func TestProxyHandlerMultipleConnections(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())

	done := make(chan bool, 3)
	for i := 0; i < 3; i++ {
		go func() {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
			proxy.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			done <- true
		}()
	}

	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestProxyHandlerInvalidTargetURL(t *testing.T) {
	proxy := NewProxyHandler("ht!tp://invalid url with spaces", newTestLogger())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestProxyHandlerMissingScheme(t *testing.T) {
	proxy := NewProxyHandler("localhost:8080", newTestLogger())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestProxyHandlerUnsupportedScheme(t *testing.T) {
	proxy := NewProxyHandler("ftp://example.com", newTestLogger())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestProxyHandlerMissingHost(t *testing.T) {
	proxy := NewProxyHandler("http://", newTestLogger())

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusBadGateway, recorder.Code)
}

func TestProxyHandlerExistingXForwardedHeadersPreserved(t *testing.T) {
	capturedHeaders := map[string]string{}
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders[HeaderXForwardedFor] = r.Header.Get(HeaderXForwardedFor)
		capturedHeaders[HeaderXForwardedProto] = r.Header.Get(HeaderXForwardedProto)
		capturedHeaders[HeaderXForwardedHost] = r.Header.Get(HeaderXForwardedHost)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)
	request.Header.Set(HeaderXForwardedProto, "https")
	request.RemoteAddr = "10.0.0.1:54321"

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "https", capturedHeaders[HeaderXForwardedProto])
	assert.Contains(t, capturedHeaders[HeaderXForwardedFor], "10.0.0.1")
}

func TestProxyHandlerQueryParametersPreserved(t *testing.T) {
	queryParams := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryParams = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/search?q=test&page=2&limit=50", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, queryParams, "q=test")
	assert.Contains(t, queryParams, "page=2")
	assert.Contains(t, queryParams, "limit=50")
}

func TestProxyHandlerEmptyResponse(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "http://localhost:8080/api/test", nil)

	proxy.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Code)
	assert.Equal(t, "", recorder.Body.String())
}

// --- ActivityRecorder integration tests ---

// TestProxyHandlerWithActivityRecorderRecordsActivityOnSuccessfulRequest verifies that
// RecordActivity is called with the correct routeID on every successful request.
func TestProxyHandlerWithActivityRecorderRecordsActivityOnSuccessfulRequest(t *testing.T) {
	// Given: a backend that returns 200 and a proxy with an ActivityRecorder
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	recorder := &mockActivityRecorder{}
	proxy := NewProxyHandler(backend.URL, newTestLogger(), WithActivityRecorder(recorder, "my-route"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// When: a request is served
	proxy.ServeHTTP(w, r)

	// Then: RecordActivity was called exactly once with the correct route ID
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, recorder.callCount())
	assert.Equal(t, "my-route", recorder.lastRouteID())
}

// TestProxyHandlerWithActivityRecorderRecordsActivityOnBackendError verifies that
// RecordActivity is called even when the backend is unreachable (502 Bad Gateway).
func TestProxyHandlerWithActivityRecorderRecordsActivityOnBackendError(t *testing.T) {
	// Given: an unreachable backend and a proxy with an ActivityRecorder
	recorder := &mockActivityRecorder{}
	proxy := NewProxyHandler("http://127.0.0.1:1", newTestLogger(), WithActivityRecorder(recorder, "error-route"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// When: a request is served (backend will fail)
	proxy.ServeHTTP(w, r)

	// Then: RecordActivity was called before proxying (even on error)
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Equal(t, 1, recorder.callCount())
	assert.Equal(t, "error-route", recorder.lastRouteID())
}

// TestProxyHandlerWithActivityRecorderRecordsBeforeProxying verifies that
// RecordActivity is called before the request reaches the backend.
func TestProxyHandlerWithActivityRecorderRecordsBeforeProxying(t *testing.T) {
	// Given: a backend that checks a flag set by RecordActivity
	activityRecordedBeforeBackend := false
	recorder := &mockActivityRecorder{}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// When this handler runs, RecordActivity must already have been called
		activityRecordedBeforeBackend = recorder.callCount() > 0
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger(), WithActivityRecorder(recorder, "order-route"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// When: a request is served
	proxy.ServeHTTP(w, r)

	// Then: activity was recorded before the backend handled the request
	assert.True(t, activityRecordedBeforeBackend, "RecordActivity must be called before proxying")
}

// TestProxyHandlerWithActivityRecorderRecordsOnMultipleRequests verifies that
// RecordActivity is called once per request, not just once total.
func TestProxyHandlerWithActivityRecorderRecordsOnMultipleRequests(t *testing.T) {
	// Given: a backend and a proxy with an ActivityRecorder
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	recorder := &mockActivityRecorder{}
	proxy := NewProxyHandler(backend.URL, newTestLogger(), WithActivityRecorder(recorder, "multi-route"))

	// When: three requests are served
	for range 3 {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		proxy.ServeHTTP(w, r)
	}

	// Then: RecordActivity was called exactly three times
	assert.Equal(t, 3, recorder.callCount())
}

// TestProxyHandlerWithoutActivityRecorderWorksNormally verifies that when no
// ActivityRecorder is provided, the proxy handler behaves exactly as before.
func TestProxyHandlerWithoutActivityRecorderWorksNormally(t *testing.T) {
	// Given: a backend and a proxy without an ActivityRecorder
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	proxy := NewProxyHandler(backend.URL, newTestLogger())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// When: a request is served (no recorder, should not panic)
	require.NotPanics(t, func() {
		proxy.ServeHTTP(w, r)
	})

	// Then: request handled normally
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

// TestProxyHandlerWithActivityRecorderInvalidURLStillRecords verifies that
// RecordActivity is called even when the target URL is invalid and the handler
// returns 502 immediately (before proxying).
func TestProxyHandlerWithActivityRecorderInvalidURLStillRecords(t *testing.T) {
	// Given: an invalid target URL and an ActivityRecorder
	recorder := &mockActivityRecorder{}
	proxy := NewProxyHandler("http://", newTestLogger(), WithActivityRecorder(recorder, "invalid-route"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	// When: a request is served
	proxy.ServeHTTP(w, r)

	// Then: 502 is returned (invalid host) and RecordActivity was still called
	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Equal(t, 1, recorder.callCount())
	assert.Equal(t, "invalid-route", recorder.lastRouteID())
}
