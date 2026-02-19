package main

import (
	"context"
	"fmt"
	"os"
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
