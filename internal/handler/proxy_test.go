package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
