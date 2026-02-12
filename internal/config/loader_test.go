package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfigYAML is a complete valid configuration used across multiple tests
const validConfigYAML = `settings:
  gwaihir:
    url: "http://gwaihir-service.ai.svc.cluster.local"
    apiKey: "test-api-key"
    timeout: 5s

  logging:
    level: info
    format: json

  observability:
    healthCheck:
      enabled: true
      port: 2111
    metrics:
      enabled: true
      port: 2112

servers:
  saruman:
    mac: "AA:BB:CC:DD:EE:FF"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s
    sleepOnLan:
      enabled: true
      endpoint: "http://saruman.from-gondor.com:8000/sleep"
      authToken: "test-token"
      idleTimeout: 5m
    healthCheck:
      endpoint: "http://saruman.from-gondor.com:8000/status"
      interval: 2s
      timeout: 2s

routes:
  - name: ollama
    listen: 11434
    upstream: "http://saruman.from-gondor.com:11434"
    server: saruman
  - name: marker
    listen: 8080
    upstream: "http://saruman.from-gondor.com:8080"
    server: saruman
`

// TestLoadValidConfig validates that a valid config file loads successfully
func TestLoadValidConfig(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called with the valid file path
	config, err := Load(tmpFile)

	// Then: The configuration is loaded successfully without error
	require.NoError(t, err, "Load should not return an error for valid YAML")
	require.NotNil(t, config, "Config should not be nil")
}

// TestLoadValidConfigParsesGlobalSettings validates global settings parsing
func TestLoadValidConfigParsesGlobalSettings(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)

	// Then: Gwaihir configuration is parsed correctly
	assert.Equal(t, "http://gwaihir-service.ai.svc.cluster.local", config.Settings.Gwaihir.URL, "Gwaihir URL should match")
	assert.Equal(t, "test-api-key", config.Settings.Gwaihir.APIKey, "Gwaihir API key should match")
	assert.Equal(t, 5*time.Second, config.Settings.Gwaihir.Timeout, "Gwaihir timeout should match")

	// Then: Logging configuration is parsed correctly
	assert.Equal(t, "info", config.Settings.Logging.Level, "Logging level should be info")
	assert.Equal(t, "json", config.Settings.Logging.Format, "Logging format should be json")
}

// TestLoadValidConfigParsesObservability validates observability settings parsing
func TestLoadValidConfigParsesObservability(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)

	// Then: Health check configuration is parsed correctly
	assert.True(t, config.Settings.Observability.HealthCheck.Enabled, "Health check should be enabled")
	assert.Equal(t, 2111, config.Settings.Observability.HealthCheck.Port, "Health check port should be 2111")

	// Then: Metrics configuration is parsed correctly
	assert.True(t, config.Settings.Observability.Metrics.Enabled, "Metrics should be enabled")
	assert.Equal(t, 2112, config.Settings.Observability.Metrics.Port, "Metrics port should be 2112")
}

// TestLoadValidConfigParsesServers validates server configuration parsing
func TestLoadValidConfigParsesServers(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)

	// Then: Server 'saruman' exists with correct basic configuration
	require.Contains(t, config.Servers, "saruman", "Should have server 'saruman'")
	saruman := config.Servers["saruman"]
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", saruman.MAC, "Server MAC should match")
	assert.Equal(t, "192.168.1.255", saruman.Broadcast, "Server broadcast should match")
}

// TestLoadValidConfigParsesServerWakeOnLan validates WakeOnLan configuration parsing
func TestLoadValidConfigParsesServerWakeOnLan(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	// Then: WakeOnLan configuration is parsed correctly
	assert.True(t, saruman.WakeOnLan.Enabled, "WakeOnLan should be enabled")
	assert.Equal(t, 60*time.Second, saruman.WakeOnLan.Timeout, "WakeOnLan timeout should be 60s")
	assert.Equal(t, 5*time.Second, saruman.WakeOnLan.Debounce, "WakeOnLan debounce should be 5s")
}

// TestLoadValidConfigParsesServerSleepOnLan validates SleepOnLan configuration parsing
func TestLoadValidConfigParsesServerSleepOnLan(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	// Then: SleepOnLan configuration is parsed correctly
	assert.True(t, saruman.SleepOnLan.Enabled, "SleepOnLan should be enabled")
	assert.Equal(t, "http://saruman.from-gondor.com:8000/sleep", saruman.SleepOnLan.Endpoint, "SleepOnLan endpoint should match")
	assert.Equal(t, "test-token", saruman.SleepOnLan.AuthToken, "SleepOnLan auth token should match")
	assert.Equal(t, 5*time.Minute, saruman.SleepOnLan.IdleTimeout, "SleepOnLan idle timeout should be 5m")
}

// TestLoadValidConfigParsesServerHealthCheck validates server health check configuration parsing
func TestLoadValidConfigParsesServerHealthCheck(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	// Then: Server HealthCheck configuration is parsed correctly
	assert.Equal(t, "http://saruman.from-gondor.com:8000/status", saruman.HealthCheck.Endpoint, "Server health check endpoint should match")
	assert.Equal(t, 2*time.Second, saruman.HealthCheck.Interval, "Server health check interval should be 2s")
	assert.Equal(t, 2*time.Second, saruman.HealthCheck.Timeout, "Server health check timeout should be 2s")
}

// TestLoadValidConfigParsesRoutes validates route configuration parsing
func TestLoadValidConfigParsesRoutes(t *testing.T) {
	// Given: A valid YAML configuration file
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called
	config, err := Load(tmpFile)

	// Then: No error occurs
	require.NoError(t, err)
	require.NotNil(t, config)

	// Then: Routes configuration has correct count
	require.Len(t, config.Routes, 2, "Should have exactly 2 routes")

	// Then: First route (ollama) is parsed correctly
	assert.Equal(t, "ollama", config.Routes[0].Name, "First route name should be ollama")
	assert.Equal(t, 11434, config.Routes[0].Listen, "First route listen port should be 11434")
	assert.Equal(t, "http://saruman.from-gondor.com:11434", config.Routes[0].Upstream, "First route upstream should match")
	assert.Equal(t, "saruman", config.Routes[0].Server, "First route server should be saruman")

	// Then: Second route (marker) is parsed correctly
	assert.Equal(t, "marker", config.Routes[1].Name, "Second route name should be marker")
	assert.Equal(t, 8080, config.Routes[1].Listen, "Second route listen port should be 8080")
	assert.Equal(t, "http://saruman.from-gondor.com:8080", config.Routes[1].Upstream, "Second route upstream should match")
	assert.Equal(t, "saruman", config.Routes[1].Server, "Second route server should be saruman")
}

// TestLoadInvalidYAML confirms error handling for malformed YAML
func TestLoadInvalidYAML(t *testing.T) {
	// Given: A malformed YAML configuration file
	invalidYAML := `settings:
  gwaihir:
    url: "http://example.com"
  invalid_indentation:
wrong indentation here
no colon for this line
`
	tmpFile := createTempConfigFile(t, invalidYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: The Load function is called with the malformed file path
	config, err := Load(tmpFile)

	// Then: An error is returned
	assert.Error(t, err, "Load should return an error for malformed YAML")
	assert.Nil(t, config, "Config should be nil when error occurs")
	assert.Contains(t, err.Error(), "failed to parse", "Error message should indicate parsing failure")
}

// TestLoadFileNotFound confirms error handling for missing files
func TestLoadFileNotFound(t *testing.T) {
	// Given: A non-existent file path
	nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	// When: The Load function is called with the non-existent file path
	config, err := Load(nonExistentPath)

	// Then: An error is returned
	assert.Error(t, err, "Load should return an error for missing file")
	assert.Nil(t, config, "Config should be nil when error occurs")
	assert.Contains(t, err.Error(), "failed to read", "Error message should indicate read failure")
}

// Helper function to create temporary config files for testing
func createTempConfigFile(t *testing.T, content string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err, "Failed to write to temp file")

	err = tmpFile.Close()
	require.NoError(t, err, "Failed to close temp file")

	return tmpFile.Name()
}
