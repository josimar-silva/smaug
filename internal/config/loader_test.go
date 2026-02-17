package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestLoadValidConfig(t *testing.T) {
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	config, err := Load(tmpFile)

	require.NoError(t, err, "Load should not return an error for valid YAML")
	require.NotNil(t, config, "Config should not be nil")
}

func TestLoadValidConfigParsesGlobalSettings(t *testing.T) {
	config := aValidConfigFor(t)

	assert.Equal(t, "http://gwaihir-service.ai.svc.cluster.local", config.Settings.Gwaihir.URL, "Gwaihir URL should match")
	assert.Equal(t, "test-api-key", config.Settings.Gwaihir.APIKey.Value(), "Gwaihir API key should match")
	assert.Equal(t, 5*time.Second, config.Settings.Gwaihir.Timeout, "Gwaihir timeout should match")

	assert.Equal(t, "info", config.Settings.Logging.Level, "Logging level should be info")
	assert.Equal(t, "json", config.Settings.Logging.Format, "Logging format should be json")
}

func TestLoadValidConfigParsesObservability(t *testing.T) {
	config := aValidConfigFor(t)

	assert.True(t, config.Settings.Observability.HealthCheck.Enabled, "Health check should be enabled")
	assert.Equal(t, 2111, config.Settings.Observability.HealthCheck.Port, "Health check port should be 2111")

	assert.True(t, config.Settings.Observability.Metrics.Enabled, "Metrics should be enabled")
	assert.Equal(t, 2112, config.Settings.Observability.Metrics.Port, "Metrics port should be 2112")
}

func TestLoadValidConfigParsesServers(t *testing.T) {
	config := aValidConfigFor(t)

	require.Contains(t, config.Servers, "saruman", "Should have server 'saruman'")
	saruman := config.Servers["saruman"]

	assert.NotNil(t, saruman, "Server should be available")
}

func TestLoadValidConfigParsesServerWakeOnLan(t *testing.T) {
	config := aValidConfigFor(t)

	require.NotNil(t, config)
	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	assert.True(t, saruman.WakeOnLan.Enabled, "WakeOnLan should be enabled")
	assert.Equal(t, 60*time.Second, saruman.WakeOnLan.Timeout, "WakeOnLan timeout should be 60s")
	assert.Equal(t, 5*time.Second, saruman.WakeOnLan.Debounce, "WakeOnLan debounce should be 5s")
}

func TestLoadValidConfigParsesServerSleepOnLan(t *testing.T) {
	config := aValidConfigFor(t)

	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	assert.True(t, saruman.SleepOnLan.Enabled, "SleepOnLan should be enabled")
	assert.Equal(t, "http://saruman.from-gondor.com:8000/sleep", saruman.SleepOnLan.Endpoint, "SleepOnLan endpoint should match")
	assert.Equal(t, "test-token", saruman.SleepOnLan.AuthToken.Value(), "SleepOnLan auth token should match")
	assert.Equal(t, 5*time.Minute, saruman.SleepOnLan.IdleTimeout, "SleepOnLan idle timeout should be 5m")
}

func TestLoadValidConfigParsesServerHealthCheck(t *testing.T) {
	config := aValidConfigFor(t)

	require.Contains(t, config.Servers, "saruman")
	saruman := config.Servers["saruman"]

	assert.Equal(t, "http://saruman.from-gondor.com:8000/status", saruman.HealthCheck.Endpoint, "Server health check endpoint should match")
	assert.Equal(t, 2*time.Second, saruman.HealthCheck.Interval, "Server health check interval should be 2s")
	assert.Equal(t, 2*time.Second, saruman.HealthCheck.Timeout, "Server health check timeout should be 2s")
}

func TestLoadValidConfigParsesRoutes(t *testing.T) {
	config := aValidConfigFor(t)

	require.Len(t, config.Routes, 2, "Should have exactly 2 routes")

	assert.Equal(t, "ollama", config.Routes[0].Name, "First route name should be ollama")
	assert.Equal(t, 11434, config.Routes[0].Listen, "First route listen port should be 11434")
	assert.Equal(t, "http://saruman.from-gondor.com:11434", config.Routes[0].Upstream, "First route upstream should match")
	assert.Equal(t, "saruman", config.Routes[0].Server, "First route server should be saruman")

	assert.Equal(t, "marker", config.Routes[1].Name, "Second route name should be marker")
	assert.Equal(t, 8080, config.Routes[1].Listen, "Second route listen port should be 8080")
	assert.Equal(t, "http://saruman.from-gondor.com:8080", config.Routes[1].Upstream, "Second route upstream should match")
	assert.Equal(t, "saruman", config.Routes[1].Server, "Second route server should be saruman")
}

func TestLoadValidConfigRedactsSecretsInStringFormat(t *testing.T) {
	config := aValidConfigFor(t)

	fmtOutput := fmt.Sprintf("%+v", config)

	assert.Contains(t, fmtOutput, "***REDACTED***")
	assert.NotContains(t, fmtOutput, "test-api-key")
	assert.NotContains(t, fmtOutput, "test-token")
}

func TestLoadValidConfigRedactsSecretsInLogs(t *testing.T) {
	config := aValidConfigFor(t)

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("config loaded",
		slog.Any("gwaihir", config.Settings.Gwaihir),
		slog.Any("sleepOnLan", config.Servers["test-server"].SleepOnLan),
	)
	slogOutput := buf.String()

	assert.Contains(t, slogOutput, "***REDACTED***")
	assert.NotContains(t, slogOutput, "test-api-key")
	assert.NotContains(t, slogOutput, "test-token")
}

func TestLoadValidConfigRedactsSecretsWhenMarshaledToJson(t *testing.T) {
	config := aValidConfigFor(t)

	jsonBytes, err := json.Marshal(config)
	require.NoError(t, err)
	jsonOutput := string(jsonBytes)

	assert.Contains(t, jsonOutput, "***REDACTED***")
	assert.NotContains(t, jsonOutput, "test-api-key")
	assert.NotContains(t, jsonOutput, "test-token")
}

func TestLoadInvalidYAML(t *testing.T) {
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

	config, err := Load(tmpFile)

	assert.Error(t, err, "Load should return an error for malformed YAML")
	assert.Nil(t, config, "Config should be nil when error occurs")
	assert.Contains(t, err.Error(), "failed to parse", "Error message should indicate parsing failure")
}

func TestLoadFileNotFound(t *testing.T) {
	nonExistentPath := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	config, err := Load(nonExistentPath)

	assert.Error(t, err, "Load should return an error for missing file")
	assert.Nil(t, config, "Config should be nil when error occurs")
	assert.Contains(t, err.Error(), "failed to read", "Error message should indicate read failure")
}

func TestLoadValidationFailure(t *testing.T) {
	invalidConfigYAML := `settings:
  gwaihir:
    url: "invalid-url"
    timeout: -5s
  observability:
    healthCheck:
      enabled: true
      port: 0

servers:

routes:
  - name: ""
    listen: 99999
    upstream: "not-a-url"
`
	tmpFile := createTempConfigFile(t, invalidConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	config, err := Load(tmpFile)

	assert.Error(t, err, "Load should return an error for invalid config")
	assert.Nil(t, config, "Config should be nil when validation fails")
	assert.Contains(t, err.Error(), "config validation failed", "Error message should indicate validation failure")
}

func aValidConfigFor(t *testing.T) *Config {
	tmpFile := createTempConfigFile(t, validConfigYAML)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	config, err := Load(tmpFile)
	require.NoError(t, err, "Load should not return an error for valid YAML")
	require.NotNil(t, config, "Config should not be nil")
	return config
}

func TestLoadWithEnvironmentVariableSubstitution(t *testing.T) {
	// Given: A config file with environment variable references
	t.Setenv("GWAIHIR_API_KEY", "env-api-key-123")
	t.Setenv("GWAIHIR_URL", "http://gwaihir.example.com")

	configWithVars := `settings:
  gwaihir:
    url: "${GWAIHIR_URL}"
    apiKey: "${GWAIHIR_API_KEY}"
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
    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s
    sleepOnLan:
      enabled: false
    healthCheck:
      endpoint: "http://saruman.example.com:8000/status"
      interval: 2s
      timeout: 2s

routes: []
`

	tmpFile := createTempConfigFile(t, configWithVars)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: Loading the config
	config, err := Load(tmpFile)

	// Then: Environment variables are substituted in nested fields
	require.NoError(t, err, "Load should succeed with env var substitution")
	require.NotNil(t, config)
	assert.Equal(t, "http://gwaihir.example.com", config.Settings.Gwaihir.URL)
	assert.Equal(t, "env-api-key-123", config.Settings.Gwaihir.APIKey.Value())
}

func TestLoadWithEnvironmentVariableDefaultValues(t *testing.T) {
	// Given: A config file with default values for missing environment variables
	configWithDefaults := `settings:
  gwaihir:
    url: "${MISSING_GWAIHIR_URL:-http://localhost:8000}"
    apiKey: "${MISSING_GWAIHIR_KEY:-default-key}"
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
    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s
    sleepOnLan:
      enabled: false
    healthCheck:
      endpoint: "http://saruman.example.com:8000/status"
      interval: 2s
      timeout: 2s

routes: []
`

	tmpFile := createTempConfigFile(t, configWithDefaults)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: Loading the config with missing environment variables
	config, err := Load(tmpFile)

	// Then: Default values are used for missing variables
	require.NoError(t, err, "Load should succeed with default values")
	require.NotNil(t, config)
	assert.Equal(t, "http://localhost:8000", config.Settings.Gwaihir.URL)
	assert.Equal(t, "default-key", config.Settings.Gwaihir.APIKey.Value())
}

func TestLoadWithMixedEnvironmentVariables(t *testing.T) {
	// Given: A config with some vars set and some missing (using defaults)
	t.Setenv("HEALTH_ENDPOINT", "http://server.example.com:8000/status")

	configMixed := `settings:
  gwaihir:
    url: "${MISSING_URL:-http://default.local}"
    apiKey: "static-key"
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
  server1:
    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s
    sleepOnLan:
      enabled: false
    healthCheck:
      endpoint: "${HEALTH_ENDPOINT}"
      interval: "${HEALTH_INTERVAL:-2s}"
      timeout: 2s

routes: []
`

	tmpFile := createTempConfigFile(t, configMixed)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: Loading the config
	config, err := Load(tmpFile)

	// Then: Mix of env vars and defaults are applied correctly
	require.NoError(t, err, "Load should succeed with mixed env vars")
	require.NotNil(t, config)
	require.Contains(t, config.Servers, "server1")

	server := config.Servers["server1"]
	assert.Equal(t, "http://server.example.com:8000/status", server.HealthCheck.Endpoint)
	assert.Equal(t, "2s", server.HealthCheck.Interval.String())

	assert.Equal(t, "http://default.local", config.Settings.Gwaihir.URL)
}

func TestLoadSubstitutionInMultipleNestedLevels(t *testing.T) {
	// Given: A config with environment variables in deeply nested fields
	t.Setenv("SLEEP_ENDPOINT", "http://sleep.example.com/api")
	t.Setenv("SLEEP_TOKEN", "secret-sleep-token")
	t.Setenv("HEALTH_ENDPOINT", "http://health.example.com/check")

	configNested := `settings:
  gwaihir:
    url: "http://localhost"
    apiKey: "key"
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
  server1:
    wakeOnLan:
      enabled: true
      timeout: 60s
      debounce: 5s
    sleepOnLan:
      enabled: true
      endpoint: "${SLEEP_ENDPOINT}"
      authToken: "${SLEEP_TOKEN}"
      idleTimeout: 5m
    healthCheck:
      endpoint: "${HEALTH_ENDPOINT}"
      interval: 2s
      timeout: 2s

routes: []
`

	tmpFile := createTempConfigFile(t, configNested)
	defer func() {
		_ = os.Remove(tmpFile)
	}()

	// When: Loading the config
	config, err := Load(tmpFile)

	// Then: Substitution works at multiple nesting levels
	require.NoError(t, err, "Load should succeed with nested substitution")
	require.NotNil(t, config)
	require.Contains(t, config.Servers, "server1")
	server := config.Servers["server1"]
	assert.Equal(t, "http://sleep.example.com/api", server.SleepOnLan.Endpoint)
	assert.Equal(t, "secret-sleep-token", server.SleepOnLan.AuthToken.Value())
	assert.Equal(t, "http://health.example.com/check", server.HealthCheck.Endpoint)
}

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
