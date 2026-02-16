package health_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/management/health"
)

// MockRouteStatusProvider is a mock implementation of RouteStatusProvider for testing.
type MockRouteStatusProvider struct {
	activeRouteCount int
}

func (m *MockRouteStatusProvider) GetActiveRouteCount() int {
	return m.activeRouteCount
}

// newTestLogger creates a logger for testing that writes to a buffer.
func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, &bytes.Buffer{})
}

func TestHealthHandlerReturnsHealthStatus(t *testing.T) {
	// Given: a health handler with mock dependencies
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 5}
	versionInfo := health.VersionInfo{
		Version:   "0.1.0-SNAPSHOT",
		BuildTime: "2026-02-16T12:00:00Z",
		GitCommit: "abc123",
	}
	log := newTestLogger()
	startTime := time.Now().Add(-1 * time.Hour)

	handler := health.NewHealthHandler(mockProvider, versionInfo, log, startTime)

	// When: making a GET request to the handler
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response is 200 OK with correct JSON
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response health.ApplicationHealth
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.Equal(t, "0.1.0-SNAPSHOT", response.Version)
	assert.Equal(t, 5, response.ActiveRoutes)
	assert.NotEmpty(t, response.Uptime)
}

func TestHealthHandlerReturnsCorrectUptime(t *testing.T) {
	// Given: a health handler with known start time
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 2}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()
	startTime := time.Now().Add(-2 * time.Hour)

	handler := health.NewHealthHandler(mockProvider, versionInfo, log, startTime)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: uptime is approximately 2 hours
	var response health.ApplicationHealth
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	// Uptime should be around "2h0m" (allowing for test execution time)
	assert.Contains(t, response.Uptime, "2h")
}

func TestHealthHandlerHandlesZeroActiveRoutes(t *testing.T) {
	// Given: a health handler with zero active routes
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 0}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()
	startTime := time.Now()

	handler := health.NewHealthHandler(mockProvider, versionInfo, log, startTime)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response shows zero active routes
	var response health.ApplicationHealth
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.ActiveRoutes)
	assert.Equal(t, "healthy", response.Status)
}

func TestLiveHandlerReturnsAliveStatus(t *testing.T) {
	// Given: a liveness handler
	log := newTestLogger()
	handler := health.NewLiveHandler(log)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response is 200 OK with alive status
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response health.LivenessResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "alive", response.Status)
}

func TestReadyHandlerReturnsReadyWhenRoutesActive(t *testing.T) {
	// Given: a readiness handler with active routes
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 3}
	log := newTestLogger()
	handler := health.NewReadyHandler(mockProvider, log)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response is 200 OK with ready status
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response health.ReadinessResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response.Status)
	assert.Equal(t, 3, response.ActiveRoutes)
}

func TestReadyHandlerReturnsNotReadyWhenNoRoutesActive(t *testing.T) {
	// Given: a readiness handler with no active routes
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 0}
	log := newTestLogger()
	handler := health.NewReadyHandler(mockProvider, log)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response is 503 Service Unavailable
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response health.ReadinessResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
	assert.Equal(t, 0, response.ActiveRoutes)
}

func TestVersionHandlerReturnsVersionInfo(t *testing.T) {
	// Given: a version handler with version information
	versionInfo := health.VersionInfo{
		Version:   "0.1.0-SNAPSHOT",
		BuildTime: "2026-02-16T12:00:00Z",
		GitCommit: "abc123def456",
	}
	log := newTestLogger()
	handler := health.NewVersionHandler(versionInfo, log)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response is 200 OK with version details
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response health.VersionResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "0.1.0-SNAPSHOT", response.Version)
	assert.Equal(t, "2026-02-16T12:00:00Z", response.BuildTime)
	assert.Equal(t, "abc123def456", response.GitCommit)
}

func TestVersionHandlerHandlesUnknownBuildInfo(t *testing.T) {
	// Given: a version handler with unknown build information
	versionInfo := health.VersionInfo{
		Version:   "0.1.0-SNAPSHOT",
		BuildTime: "unknown",
		GitCommit: "unknown",
	}
	log := newTestLogger()
	handler := health.NewVersionHandler(versionInfo, log)

	// When: making a GET request
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Then: response includes unknown values
	var response health.VersionResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "unknown", response.BuildTime)
	assert.Equal(t, "unknown", response.GitCommit)
}

func TestHandlersOnlyAcceptGETMethod(t *testing.T) {
	tests := []struct {
		name    string
		handler http.Handler
		path    string
	}{
		{
			name:    "health handler rejects POST",
			handler: health.NewHealthHandler(&MockRouteStatusProvider{}, health.VersionInfo{}, newTestLogger(), time.Now()),
			path:    "/health",
		},
		{
			name:    "live handler rejects POST",
			handler: health.NewLiveHandler(newTestLogger()),
			path:    "/live",
		},
		{
			name:    "ready handler rejects POST",
			handler: health.NewReadyHandler(&MockRouteStatusProvider{}, newTestLogger()),
			path:    "/ready",
		},
		{
			name:    "version handler rejects POST",
			handler: health.NewVersionHandler(health.VersionInfo{}, newTestLogger()),
			path:    "/version",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: making a POST request
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, req)

			// Then: response is 405 Method Not Allowed
			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
		})
	}
}

func TestHandlersConcurrentRequests(t *testing.T) {
	// Given: a health handler
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 10}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()
	handler := health.NewHealthHandler(mockProvider, versionInfo, log, time.Now())

	// When: making concurrent requests
	const numRequests = 100
	done := make(chan bool, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
			done <- true
		}()
	}

	// Then: all requests complete successfully
	for i := 0; i < numRequests; i++ {
		<-done
	}
}
