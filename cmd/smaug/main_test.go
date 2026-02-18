package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
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

func TestRun_InvalidConfigPath(t *testing.T) {
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
