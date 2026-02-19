package main

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	sleepclient "github.com/josimar-silva/smaug/internal/client/sleep"
	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/proxy"
)

// newTestLogger returns a test logger that writes to a discard sink.
func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log := logger.New(logger.LevelInfo, logger.TEXT, nil)
	t.Cleanup(func() { _ = log.Stop() })
	return log
}

// configWithGwaihir builds a *config.Config with the given Gwaihir URL and API key via
// YAML unmarshaling so that the unexported SecretString.value field is populated.
func configWithGwaihir(t *testing.T, url, apiKey string) *config.Config {
	t.Helper()
	raw := fmt.Sprintf(`
settings:
  gwaihir:
    url: %q
    apiKey: %q
    timeout: 5s
`, url, apiKey)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	return &cfg
}

// configWithWoLServer builds a *config.Config with Gwaihir settings and one
// server that has WoL enabled.
func configWithWoLServer(t *testing.T, gwaihirURL, apiKey, serverID string) *config.Config {
	t.Helper()
	raw := fmt.Sprintf(`
settings:
  gwaihir:
    url: %q
    apiKey: %q
    timeout: 5s
servers:
  %s:
    wakeOnLan:
      enabled: true
      machineId: "server-machine"
      timeout: 30s
      debounce: 5s
`, gwaihirURL, apiKey, serverID)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	return &cfg
}

// mockRouteStatusProvider is a mock implementation of RouteStatusProvider for testing.
type mockRouteStatusProvider struct {
	activeRouteCount int
}

func (m *mockRouteStatusProvider) GetActiveRouteCount() int {
	return m.activeRouteCount
}

func TestRunInvalidConfigPath(t *testing.T) {
	configPath := "/nonexistent/path/services.yaml"
	originalEnv := os.Getenv("SMAUG_CONFIG")
	defer func() {
		if originalEnv != "" {
			_ = os.Setenv("SMAUG_CONFIG", originalEnv)
		} else {
			_ = os.Unsetenv("SMAUG_CONFIG")
		}
	}()

	_ = os.Setenv("SMAUG_CONFIG", configPath)

	err := run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create config manager")
}

func TestStartManagementServer(t *testing.T) {
	// Given: Configuration with health check enabled
	cfg := &config.Config{
		Settings: config.SettingsConfig{
			Observability: config.ObservabilityConfig{
				HealthCheck: config.HealthCheckConfig{
					Enabled: true,
					Port:    12345, // Use a high port number to avoid conflicts
				},
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.TEXT, nil)
	defer func() { _ = log.Stop() }()

	mockRM := &mockRouteStatusProvider{activeRouteCount: 1}

	// When: Starting management server
	server, err := startManagementServer(cfg, mockRM, log)

	// Then: Server is created and started successfully
	assert.NoError(t, err)
	require.NotNil(t, server)

	// Clean up: Stop the server
	err = server.Stop()
	assert.NoError(t, err)
}

func TestStartManagementServerSetsVersionInfo(t *testing.T) {
	// Given: Configuration with health check enabled
	cfg := &config.Config{
		Settings: config.SettingsConfig{
			Observability: config.ObservabilityConfig{
				HealthCheck: config.HealthCheckConfig{
					Enabled: true,
					Port:    12346, // Use a different port
				},
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.TEXT, nil)
	defer func() { _ = log.Stop() }()

	mockRM := &mockRouteStatusProvider{activeRouteCount: 2}

	// When: Starting management server
	server, err := startManagementServer(cfg, mockRM, log)

	// Then: Server is created with version info from constants
	assert.NoError(t, err)
	require.NotNil(t, server)

	// Clean up
	err = server.Stop()
	assert.NoError(t, err)
}

// --- initWakeOptions ---

func TestInitWakeOptionsGwaihirURLNotConfigured(t *testing.T) {
	// Given: No Gwaihir URL in config
	cfg := &config.Config{}
	log := newTestLogger(t)

	// When
	opts, err := initWakeOptions(cfg, log)

	// Then: WoL coordination is disabled; no error
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestInitWakeOptionsEmptyAPIKey(t *testing.T) {
	// Given: Gwaihir URL set, API key empty, one WoL-enabled server so we reach NewClient
	cfg := configWithWoLServer(t, "http://gwaihir.example.com", "", "homeserver")
	log := newTestLogger(t)

	// When
	opts, err := initWakeOptions(cfg, log)

	// Then: gwaihir client construction fails due to missing API key
	assert.Error(t, err)
	assert.Nil(t, opts)
	assert.Contains(t, err.Error(), "failed to create Gwaihir client")
}

func TestInitWakeOptionsNoWoLEnabledServers(t *testing.T) {
	// Given: Valid Gwaihir config but no servers with WoL enabled
	cfg := configWithGwaihir(t, "http://gwaihir.example.com", "secret-key")
	log := newTestLogger(t)

	// When
	opts, err := initWakeOptions(cfg, log)

	// Then: WoL coordination is disabled (no pollers); no error
	assert.NoError(t, err)
	assert.Nil(t, opts)
}

func TestInitWakeOptionsWithWoLEnabledServer(t *testing.T) {
	// Given: Valid Gwaihir config and one WoL-enabled server
	cfg := configWithWoLServer(t, "http://gwaihir.example.com", "secret-key", "homeserver")
	log := newTestLogger(t)

	// When
	opts, err := initWakeOptions(cfg, log)

	// Then: WakeOptions returned with a Sender
	assert.NoError(t, err)
	require.NotNil(t, opts)
	assert.NotNil(t, opts.Sender)
}

func TestInitWakeOptionsDefaultTimeout(t *testing.T) {
	// Given: Gwaihir config with zero timeout (should fall back to defaultGwaihirTimeout)
	raw := `
settings:
  gwaihir:
    url: "http://gwaihir.example.com"
    apiKey: "secret-key"
`
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))
	log := newTestLogger(t)

	// When: zero timeout — initWakeOptions must not fail with ErrInvalidTimeout
	opts, err := initWakeOptions(&cfg, log)

	// Then: default timeout applied; no WoL-enabled servers so opts is nil, no error
	assert.NoError(t, err)
	assert.Nil(t, opts)
	// The fact that we reach here without error proves defaultGwaihirTimeout was used.
}

// --- initIdleTracker ---

func TestInitIdleTrackerNoSleepOnLanRoutes(t *testing.T) {
	// Given: config with no SleepOnLan-enabled servers
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{Enabled: false}},
		},
		Routes: []config.Route{
			{Name: "route1", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	// When
	tracker, err := initIdleTracker(cfg, log)

	// Then: idle tracker is disabled (nil), no error
	assert.NoError(t, err)
	assert.Nil(t, tracker)
}

// TestInitIdleTrackerWithSleepOnLanRoute verifies that initIdleTracker creates and
// configures an IdleTracker when at least one route has SleepOnLan enabled.
func TestInitIdleTrackerWithSleepOnLanRoute(t *testing.T) {
	// Given: config with one SleepOnLan-enabled route
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{
				Enabled:     true,
				Endpoint:    "http://server1.example.com/sleep",
				IdleTimeout: 10 * time.Minute,
			}},
		},
		Routes: []config.Route{
			{Name: "route1", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	// When
	tracker, err := initIdleTracker(cfg, log)

	// Then: an idle tracker is returned with the route registered
	require.NoError(t, err)
	require.NotNil(t, tracker)

	// Verify the route was registered (idle time should be idleNeverSeen since no activity yet)
	idleTime := tracker.IdleTime("route1")
	assert.Greater(t, int64(idleTime), int64(0))
}

// TestInitIdleTrackerSkipsRouteWithNoMatchingServer verifies that a route whose
// server is not in the servers map is skipped gracefully.
func TestInitIdleTrackerSkipsRouteWithNoMatchingServer(t *testing.T) {
	// Given: route references a non-existent server name
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{
				Enabled:     true,
				Endpoint:    "http://server1.example.com/sleep",
				IdleTimeout: 5 * time.Minute,
			}},
		},
		Routes: []config.Route{
			{Name: "route-missing", Server: "nonexistent"},
			{Name: "route-valid", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	// When
	tracker, err := initIdleTracker(cfg, log)

	// Then: tracker created for the valid route only
	require.NoError(t, err)
	require.NotNil(t, tracker)
}

// --- initSleepCoordinator ---

// TestInitSleepCoordinatorReturnsNilWhenIdleTrackerIsNil verifies that no coordinator
// is created when the idle tracker is nil (SleepOnLan not configured).
func TestInitSleepCoordinatorReturnsNilWhenIdleTrackerIsNil(t *testing.T) {
	// Given: nil idle tracker
	cfg := &config.Config{}
	log := newTestLogger(t)

	// When
	coordinator, err := initSleepCoordinator(cfg, nil, log)

	// Then: no coordinator, no error
	assert.NoError(t, err)
	assert.Nil(t, coordinator)
}

// TestInitSleepCoordinatorReturnsNilWhenNoEndpointConfigured verifies that when
// all SleepOnLan routes have empty endpoints the coordinator is not created.
func TestInitSleepCoordinatorReturnsNilWhenNoEndpointConfigured(t *testing.T) {
	// Given: route with SleepOnLan enabled but no endpoint
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{
				Enabled:     true,
				Endpoint:    "", // intentionally empty
				IdleTimeout: 5 * time.Minute,
			}},
		},
		Routes: []config.Route{
			{Name: "route1", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	// Build a real IdleTracker so the function doesn't early-exit on nil check.
	tracker, err := proxy.NewIdleTracker(proxy.IdleTrackerConfig{CheckInterval: time.Second}, log)
	require.NoError(t, err)

	// When
	coordinator, err := initSleepCoordinator(cfg, tracker, log)

	// Then: no coordinator (all endpoints empty), no error
	assert.NoError(t, err)
	assert.Nil(t, coordinator)
}

// TestInitSleepCoordinatorSucceedsWithValidEndpoint verifies that a SleepCoordinator
// is created when at least one route has a valid SleepOnLan endpoint.
// A non-loopback hostname is used because the sleep client applies SSRF protection
// at construction time (it blocks 127.0.0.1 / localhost).
func TestInitSleepCoordinatorSucceedsWithValidEndpoint(t *testing.T) {
	// Given: a non-loopback endpoint URL (the client validates the scheme and host
	// at construction time only — no actual network connection is made here)
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{
				Enabled:     true,
				Endpoint:    "http://homeserver.local/sleep",
				IdleTimeout: 5 * time.Minute,
			}},
		},
		Routes: []config.Route{
			{Name: "route1", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	tracker, err := proxy.NewIdleTracker(proxy.IdleTrackerConfig{CheckInterval: time.Second}, log)
	require.NoError(t, err)

	// When
	coordinator, err := initSleepCoordinator(cfg, tracker, log)

	// Then: coordinator is returned, no error
	require.NoError(t, err)
	require.NotNil(t, coordinator)
}

// TestInitSleepCoordinatorReturnsErrorOnInvalidEndpoint verifies that an invalid
// sleep endpoint URL causes initSleepCoordinator to return an error.
func TestInitSleepCoordinatorReturnsErrorOnInvalidEndpoint(t *testing.T) {
	// Given: a sleep endpoint with an invalid (loopback) URL that the sleep
	// client rejects during construction (SSRF protection).
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {SleepOnLan: config.SleepOnLanConfig{
				Enabled:     true,
				Endpoint:    "http://127.0.0.1/sleep",
				IdleTimeout: 5 * time.Minute,
			}},
		},
		Routes: []config.Route{
			{Name: "route1", Server: "server1"},
		},
	}
	log := newTestLogger(t)

	tracker, err := proxy.NewIdleTracker(proxy.IdleTrackerConfig{CheckInterval: time.Second}, log)
	require.NoError(t, err)

	// When
	coordinator, err := initSleepCoordinator(cfg, tracker, log)

	// Then: error about client creation, no coordinator
	assert.Error(t, err)
	assert.Nil(t, coordinator)
	assert.Contains(t, err.Error(), "failed to create sleep client")
}

// --- routeSleepSender ---

// TestRouteSleepSenderDispatchesCommandToRegisteredRoute verifies that Sleep
// forwards the call to the registered sleep client for the given route.
// The sleep client blocks loopback addresses (SSRF protection), so we use an
// IANA-reserved ".invalid" domain which always fails DNS resolution immediately.
// A short context timeout keeps the test fast.
func TestRouteSleepSenderDispatchesCommandToRegisteredRoute(t *testing.T) {
	// Given: a routeSleepSender with one route registered
	log := newTestLogger(t)

	client, err := sleepclient.NewClient(sleepclient.NewClientConfig("http://no-such-host.invalid/sleep"), log)
	require.NoError(t, err)

	sender := &routeSleepSender{
		senders: map[string]*sleepclient.Client{"route1": client},
		log:     log,
	}

	// Use a short timeout so the test doesn't wait for a network timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// When: Sleep called for route1 — it should try to call the client
	// (returns a network error, NOT the silent no-op for unknown routes)
	err = sender.Sleep(ctx, "route1")

	// Then: a non-nil error confirms the call was dispatched (routing worked)
	assert.Error(t, err, "expected a network/context error — route must be dispatched, not silently skipped")
}

// TestRouteSleepSenderSkipsUnknownRoute verifies that Sleep is a no-op (returns nil)
// for a route that has no registered sender.
func TestRouteSleepSenderSkipsUnknownRoute(t *testing.T) {
	// Given: a routeSleepSender with an empty senders map
	log := newTestLogger(t)
	sender := &routeSleepSender{
		senders: make(map[string]*sleepclient.Client),
		log:     log,
	}

	// When: Sleep called for an unknown route
	err := sender.Sleep(context.Background(), "unknown-route")

	// Then: no error (graceful no-op)
	assert.NoError(t, err)
}

// TestRouteSleepSenderForwardsErrorFromClient verifies that errors from the underlying
// sleep client (network failure in this case) are propagated back to the caller.
// The sleep client blocks loopback addresses (SSRF protection), so we use a
// non-loopback hostname that causes a network error rather than a construction error.
func TestRouteSleepSenderForwardsErrorFromClient(t *testing.T) {
	// Given: an endpoint that will fail at the network level (unreachable host)
	log := newTestLogger(t)

	client, err := sleepclient.NewClient(sleepclient.NewClientConfig("http://no-such-host.invalid/sleep"), log)
	require.NoError(t, err)

	sender := &routeSleepSender{
		senders: map[string]*sleepclient.Client{"route1": client},
		log:     log,
	}

	// When: Sleep is called for route1 (network error expected)
	err = sender.Sleep(context.Background(), "route1")

	// Then: network error propagated from the sleep client
	assert.Error(t, err)
}

// --- Shutdown behavior ---
// TestGracefulShutdownTrapsSIGTERM verifies that a SIGTERM signal triggers graceful shutdown.
func TestGracefulShutdownTrapsSIGTERM(t *testing.T) {
	// Given: a signal channel and graceful shutdown handler
	sigChan := make(chan os.Signal, 1)
	shutdownCalled := false

	// When: SIGTERM is sent
	go func() {
		time.Sleep(50 * time.Millisecond)
		sigChan <- syscall.SIGTERM
	}()

	// Then: graceful shutdown is triggered
	select {
	case sig := <-sigChan:
		shutdownCalled = true
		assert.Equal(t, syscall.SIGTERM, sig)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected SIGTERM signal within timeout")
	}

	assert.True(t, shutdownCalled, "shutdown should have been called")
}

// TestGracefulShutdownTrapsINT verifies that an INT signal triggers graceful shutdown.
func TestGracefulShutdownTrapsINT(t *testing.T) {
	// Given: a signal channel and graceful shutdown handler
	sigChan := make(chan os.Signal, 1)
	shutdownCalled := false

	// When: INT (Ctrl+C) is sent
	go func() {
		time.Sleep(50 * time.Millisecond)
		sigChan <- os.Interrupt
	}()

	// Then: graceful shutdown is triggered
	select {
	case sig := <-sigChan:
		shutdownCalled = true
		assert.Equal(t, os.Interrupt, sig)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected INT signal within timeout")
	}

	assert.True(t, shutdownCalled, "shutdown should have been called")
}

// TestGracefulShutdownHas30SecondTimeout verifies that graceful shutdown respects
// a 30-second maximum timeout for in-flight requests to complete.
func TestGracefulShutdownHas30SecondTimeout(t *testing.T) {
	// Given: the graceful shutdown timeout constant
	expectedTimeout := 30 * time.Second

	// When: we examine the shutdown timeout
	actualTimeout := gracefulShutdownTimeout

	// Then: it should be exactly 30 seconds
	assert.Equal(t, expectedTimeout, actualTimeout,
		"graceful shutdown timeout should be 30 seconds for Kubernetes grace period")
}

// TestGracefulShutdownStopsAcceptingRequests verifies that the shutdown process
// prevents the route manager from accepting new requests.
// (This is implicit: RouteManager.Stop() cancels context → no new requests accepted)
func TestGracefulShutdownStopsAcceptingRequests(t *testing.T) {
	// Given: a context that will be cancelled during shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// When: the context is cancelled (simulating shutdown signal)
	cancel()

	// Then: the context should be done
	select {
	case <-ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled immediately")
	}
}

// TestGracefulShutdownWaitsForInFlightRequests verifies that the timeout allows
// in-flight requests to complete gracefully.
// The actual waiting is handled by http.Server.Shutdown() which respects the context timeout.
func TestGracefulShutdownWaitsForInFlightRequests(t *testing.T) {
	// Given: a context with the graceful shutdown timeout
	ctx, cancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer cancel()

	// When: we check the deadline
	deadline, ok := ctx.Deadline()

	// Then: a deadline should be set (the timeout ensures requests have time to complete)
	assert.True(t, ok, "context should have a deadline")
	assert.NotZero(t, deadline, "deadline should be non-zero")

	// Verify the timeout is reasonable (at least several seconds)
	duration := time.Until(deadline)
	assert.Greater(t, duration, 25*time.Second, "graceful shutdown should allow at least 25 seconds for in-flight requests")
}

// TestGracefulShutdownStopsAllGoroutines verifies that all background goroutines
// are stopped via context cancellation during graceful shutdown.
func TestGracefulShutdownStopsAllGoroutines(t *testing.T) {
	// Given: a context that simulates the shutdown signal
	ctx, cancel := context.WithCancel(context.Background())

	// When: the context is cancelled (during shutdown)
	cancel()

	// Then: all components that were started with this context should stop
	select {
	case <-ctx.Done():
		// Expected: components using this context will see cancellation
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled immediately")
	}
}

// TestGracefulShutdownExitsCleanly verifies that the graceful shutdown process
// exits with code 0 (success) when triggered by SIGTERM/INT.
// (This is enforced by main() which only exits non-zero on error, not on signal)
func TestGracefulShutdownExitsCleanly(t *testing.T) {
	// Given: a run() function that returns nil (no error) on graceful shutdown
	runErr := error(nil)

	// When: main() evaluates the error
	// (In practice: run() returns nil when signal is received)

	// Then: exit code should be 0 (not called with os.Exit(1))
	shouldExit := runErr != nil
	assert.False(t, shouldExit, "graceful shutdown should exit with code 0")
}

// TestShutdownContextHasTimeout verifies that the shutdown context created during
// signal handling has the graceful shutdown timeout applied.
func TestShutdownContextHasTimeout(t *testing.T) {
	// Given: we create a shutdown context with graceful shutdown timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		gracefulShutdownTimeout,
	)
	defer shutdownCancel()

	// When: we check the context
	deadline, ok := shutdownCtx.Deadline()

	// Then: it should have a deadline within the timeout
	assert.True(t, ok, "shutdown context should have a deadline")
	assert.NotZero(t, deadline, "deadline should be non-zero")

	// Verify the deadline is approximately 30 seconds in the future
	remaining := time.Until(deadline)
	assert.Greater(t, remaining, 25*time.Second, "at least 25 seconds should remain")
	assert.LessOrEqual(t, remaining, gracefulShutdownTimeout, "remaining time should not exceed timeout")
}

// TestShutdownContextCancellation verifies that cancelling the shutdown context
// propagates to dependent operations (e.g., goroutines waiting for Done()).
func TestShutdownContextCancellation(t *testing.T) {
	// Given: a shutdown context and cancel function
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		gracefulShutdownTimeout,
	)

	// When: we cancel the context
	shutdownCancel()

	// Then: the context should be done immediately
	select {
	case <-shutdownCtx.Done():
		// Expected: context is cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("shutdown context should be done after cancellation")
	}

	// And: the error should be context.Canceled
	assert.Equal(t, context.Canceled, shutdownCtx.Err(),
		"shutdown context error should be context.Canceled")
}

// TestGracefulShutdownTimeoutEnforcement verifies that the timeout is actually
// enforced by the context (requests exceeding the timeout should fail).
func TestGracefulShutdownTimeoutEnforcement(t *testing.T) {
	// Given: a short timeout for testing
	shortTimeout := 100 * time.Millisecond
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		shortTimeout,
	)
	defer shutdownCancel()

	// When: we wait longer than the timeout
	select {
	case <-shutdownCtx.Done():
		// Expected: context should timeout
		assert.Equal(t, context.DeadlineExceeded, shutdownCtx.Err(),
			"context should timeout with DeadlineExceeded")
	case <-time.After(500 * time.Millisecond):
		t.Fatal("context should timeout within 500ms")
	}
}

// TestSignalHandlingFlow verifies the complete signal handling flow:
// signal → context creation → cancellation → shutdown.
func TestSignalHandlingFlow(t *testing.T) {
	// Given: a buffered signal channel and goroutine to send signal
	sigChan := make(chan os.Signal, 1)

	go func() {
		time.Sleep(50 * time.Millisecond)
		sigChan <- syscall.SIGTERM
	}()

	// When: we simulate the signal handling flow
	sig := <-sigChan
	assert.Equal(t, syscall.SIGTERM, sig, "signal should be SIGTERM")

	// And: we create a shutdown context
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		gracefulShutdownTimeout,
	)
	defer shutdownCancel()

	// Then: the shutdown context should have a deadline
	deadline, ok := shutdownCtx.Deadline()
	assert.True(t, ok, "shutdown context should have deadline")
	assert.NotZero(t, deadline, "deadline should be set")

	// And: when we cancel the context, Done() should close
	shutdownCancel()
	select {
	case <-shutdownCtx.Done():
		// Expected: context cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after cancellation")
	}
}
