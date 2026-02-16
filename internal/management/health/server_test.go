package health_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/management/health"
)

func TestServerStartsOnConfiguredPort(t *testing.T) {
	// Given: a management server configured on port 9090
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9090, mockProvider, versionInfo, log)

	// When: starting the server
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Then: server is listening on port 9090
	resp, err := http.Get("http://localhost:9090/health")
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServerStopsGracefully(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9091, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	// When: stopping the server
	err = server.Stop()

	// Then: server stops without error
	require.NoError(t, err)

	// And: subsequent requests fail
	time.Sleep(50 * time.Millisecond)
	resp, err := http.Get("http://localhost:9091/health")
	if err == nil {
		_ = resp.Body.Close()
	}
	assert.Error(t, err)
}

func TestServerReturns404ForUnknownPaths(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9092, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	time.Sleep(50 * time.Millisecond)

	tests := []struct {
		name string
		path string
	}{
		{"unknown root path", "/unknown"},
		{"unknown nested path", "/api/v1/health"},
		{"invalid endpoint", "/healthz"},
		{"metrics endpoint not on this server", "/metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: requesting unknown path
			resp, err := http.Get("http://localhost:9092" + tt.path)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			// Then: response is 404 Not Found
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}

func TestServerHandlesAllKnownEndpoints(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 3}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9093, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	time.Sleep(50 * time.Millisecond)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
	}{
		{"health endpoint", "/health", http.StatusOK},
		{"liveness endpoint", "/live", http.StatusOK},
		{"readiness endpoint", "/ready", http.StatusOK},
		{"version endpoint", "/version", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: requesting known endpoint
			resp, err := http.Get("http://localhost:9093" + tt.path)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			// Then: response has expected status
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
		})
	}
}

func TestServerHandlesConcurrentRequests(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 5}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9094, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	time.Sleep(50 * time.Millisecond)

	// When: making many concurrent requests
	const numRequests = 50
	var wg sync.WaitGroup
	wg.Add(numRequests)

	for i := 0; i < numRequests; i++ {
		go func(id int) {
			defer wg.Done()

			endpoint := []string{"/health", "/live", "/ready", "/version"}[id%4]
			resp, err := http.Get("http://localhost:9094" + endpoint)
			if err != nil {
				t.Errorf("request %d failed: %v", id, err)
				return
			}
			defer func() {
				_ = resp.Body.Close()
			}()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("request %d got status %d", id, resp.StatusCode)
			}
		}(i)
	}

	// Then: all requests complete successfully
	wg.Wait()
}

func TestServerReturnsErrorWhenStartedTwice(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9095, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	time.Sleep(50 * time.Millisecond)

	// When: trying to start again
	err = server.Start(context.Background())

	// Then: error is returned
	assert.Error(t, err)
}

func TestServerReturnsErrorWhenStoppedTwice(t *testing.T) {
	// Given: a stopped management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9096, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = server.Stop()
	require.NoError(t, err)

	// When: trying to stop again
	err = server.Stop()

	// Then: error is returned
	assert.Error(t, err)
}

func TestServerCanBeStoppedBeforeStart(t *testing.T) {
	// Given: a server that was never started
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 1}
	versionInfo := health.VersionInfo{Version: "0.1.0", BuildTime: "now", GitCommit: "abc"}
	log := newTestLogger()

	server := health.NewServer(9097, mockProvider, versionInfo, log)

	// When: trying to stop
	err := server.Stop()

	// Then: error is returned
	assert.Error(t, err)
}

func TestServerResponsesContainCorrectContent(t *testing.T) {
	// Given: a running management server
	mockProvider := &MockRouteStatusProvider{activeRouteCount: 7}
	versionInfo := health.VersionInfo{
		Version:   "1.2.3",
		BuildTime: "2026-02-16T12:00:00Z",
		GitCommit: "abc123def456",
	}
	log := newTestLogger()

	server := health.NewServer(9098, mockProvider, versionInfo, log)
	err := server.Start(context.Background())
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, server.Stop())
	}()

	time.Sleep(50 * time.Millisecond)

	tests := []struct {
		name            string
		path            string
		expectedContent []string
	}{
		{
			name:            "health contains version and route count",
			path:            "/health",
			expectedContent: []string{"1.2.3", "\"activeRoutes\":7", "healthy"},
		},
		{
			name:            "live contains alive status",
			path:            "/live",
			expectedContent: []string{"alive"},
		},
		{
			name:            "ready contains route count",
			path:            "/ready",
			expectedContent: []string{"ready", "\"activeRoutes\":7"},
		},
		{
			name:            "version contains build info",
			path:            "/version",
			expectedContent: []string{"1.2.3", "2026-02-16T12:00:00Z", "abc123def456"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: requesting endpoint
			resp, err := http.Get(fmt.Sprintf("http://localhost:9098%s", tt.path))
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			bodyStr := string(body)

			// Then: response contains expected content
			for _, content := range tt.expectedContent {
				assert.True(t, strings.Contains(bodyStr, content),
					"expected body to contain '%s', got: %s", content, bodyStr)
			}
		})
	}
}
