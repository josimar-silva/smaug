package metrics

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// TestNewServerReturnsServer tests that NewServer creates a server with correct parameters.
func TestNewServerReturnsServer(t *testing.T) {
	// Given: metrics registry and logger
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()

	// When: creating a metrics server
	server := NewServer(2112, m, log)

	// Then: server should be initialized with correct values
	assert.NotNil(t, server)
	assert.False(t, server.IsRunning())
}

// TestServerStartSucceeds tests that the server starts successfully.
func TestServerStartSucceeds(t *testing.T) {
	// Given: metrics registry and logger
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log) // Port 0 for automatic assignment

	// When: starting the server
	ctx := context.Background()
	err = server.Start(ctx)

	// Then: start should succeed
	assert.NoError(t, err)
	assert.True(t, server.IsRunning())

	// Cleanup
	_ = server.Stop()
}

// TestServerStartAlreadyRunning tests that starting an already running server fails.
func TestServerStartAlreadyRunning(t *testing.T) {
	// Given: a running server
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// When: trying to start the server again
	err = server.Start(ctx)

	// Then: should return error
	assert.Error(t, err)
	assert.Equal(t, ErrServerAlreadyRunning, err)
}

// TestServerStopSucceeds tests that the server stops successfully.
func TestServerStopSucceeds(t *testing.T) {
	// Given: a running server
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// When: stopping the server
	err = server.Stop()

	// Then: stop should succeed
	assert.NoError(t, err)
	assert.False(t, server.IsRunning())
}

// TestServerStopWhenNotRunning tests that stopping a non-running server fails.
func TestServerStopWhenNotRunning(t *testing.T) {
	// Given: a non-running server
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log)

	// When: trying to stop the server
	err = server.Stop()

	// Then: should return error
	assert.Error(t, err)
	assert.Equal(t, ErrServerNotRunning, err)
}

// TestServerMetricsEndpointAccessible tests that metrics endpoint is accessible.
func TestServerMetricsEndpointAccessible(t *testing.T) {
	// Given: a running server (use specific port for HTTP access)
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(19090, m, log)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// When: accessing the metrics endpoint
	resp, err := http.Get("http://127.0.0.1:19090/metrics")

	// Then: should succeed and return 200
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain")
}

// TestServerContextCancellationShutdown tests that context cancellation triggers shutdown.
func TestServerContextCancellationShutdown(t *testing.T) {
	// Given: metrics registry and logger
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log)

	// When: starting server with a cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	err = server.Start(ctx)
	require.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Give server time to shut down
	time.Sleep(200 * time.Millisecond)

	// Then: server should be stopped
	assert.False(t, server.IsRunning())
}

// TestServerPortConfiguration tests that server uses the configured port.
func TestServerPortConfiguration(t *testing.T) {
	// Given: metrics registry with specific port
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	port := 9090
	server := NewServer(port, m, log)

	// When: accessing the server
	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = server.Stop() }()

	// Then: server should be listening on the configured port
	assert.Equal(t, port, server.port)
}

// TestServerMultipleStopCalls tests that multiple stop calls are handled gracefully.
func TestServerMultipleStopCalls(t *testing.T) {
	// Given: a running server
	m, err := metrics.New()
	require.NoError(t, err)
	log := newTestLogger()
	server := NewServer(0, m, log)

	ctx := context.Background()
	err = server.Start(ctx)
	require.NoError(t, err)

	// When: stopping the server multiple times
	err1 := server.Stop()
	err2 := server.Stop()

	// Then: first stop should succeed, second should fail
	assert.NoError(t, err1)
	assert.Error(t, err2)
	assert.Equal(t, ErrServerNotRunning, err2)
}
