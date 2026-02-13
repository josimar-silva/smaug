package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelInfo, logger.JSON, &bytes.Buffer{})
}

func assertNoReload(t *testing.T, cm *ConfigManager, timeout time.Duration) {
	t.Helper()
	select {
	case <-cm.ReloadSignal():
		t.Fatal("unexpected reload signal received")
	case <-time.After(timeout):
	}
}

func TestConfigManagerLoadAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `settings:
  gwaihir:
    url: "http://localhost:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	cm, err := NewManager(configFile, newTestLogger())
	require.NoError(t, err)
	defer func() { _ = cm.Stop() }()

	cfg := cm.GetConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "http://localhost:8080", cfg.Settings.Gwaihir.URL)
}

func TestConfigManagerWatch(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	initialContent := `settings:
  gwaihir:
    url: "http://initial:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err := os.WriteFile(configFile, []byte(initialContent), 0600)
	require.NoError(t, err)

	cm, err := NewManager(configFile, newTestLogger())
	require.NoError(t, err)
	defer func() { _ = cm.Stop() }()

	newContent := `settings:
  gwaihir:
    url: "http://updated:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err = os.WriteFile(configFile, []byte(newContent), 0600)
	require.NoError(t, err)

	select {
	case <-cm.ReloadSignal():
		cfg := cm.GetConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, "http://updated:8080", cfg.Settings.Gwaihir.URL)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}
}

func TestConfigManagerInvalidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	initialContent := `settings:
  gwaihir:
    url: "http://valid:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err := os.WriteFile(configFile, []byte(initialContent), 0600)
	require.NoError(t, err)

	cm, err := NewManager(configFile, newTestLogger())
	require.NoError(t, err)
	defer func() { _ = cm.Stop() }()

	invalidContent := `settings:
  gwaihir:
    url: [invalid yaml
`
	err = os.WriteFile(configFile, []byte(invalidContent), 0600)
	require.NoError(t, err)

	assertNoReload(t, cm, 1500*time.Millisecond)

	cfg := cm.GetConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, "http://valid:8080", cfg.Settings.Gwaihir.URL)
}

func TestConfigManagerDebounce(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	initialContent := `settings:
  gwaihir:
    url: "http://initial:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err := os.WriteFile(configFile, []byte(initialContent), 0600)
	require.NoError(t, err)

	cm, err := NewManager(configFile, newTestLogger())
	require.NoError(t, err)
	defer func() { _ = cm.Stop() }()

	for i := 0; i < 3; i++ {
		content := fmt.Sprintf(`settings:
  gwaihir:
    url: "http://update-%d:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`, i)
		err := os.WriteFile(configFile, []byte(content), 0600)
		require.NoError(t, err)
		time.Sleep(100 * time.Millisecond)
	}

	select {
	case <-cm.ReloadSignal():
		cfg := cm.GetConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, "http://update-2:8080", cfg.Settings.Gwaihir.URL)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}

	assertNoReload(t, cm, 500*time.Millisecond)
}

func TestConfigManagerStopIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `settings:
  gwaihir:
    url: "http://localhost:8080"
    apiKey: "test-key"
    timeout: 5s
  logging:
    level: info
    format: json
  observability:
    healthCheck:
      enabled: true
      port: 8080
    metrics:
      enabled: true
      port: 9090
servers:
  test:
    mac: "00:11:22:33:44:55"
    broadcast: "192.168.1.255"
    wakeOnLan:
      enabled: false
    sleepOnLan:
      enabled: false
routes: []
`
	err := os.WriteFile(configFile, []byte(configContent), 0600)
	require.NoError(t, err)

	cm, err := NewManager(configFile, newTestLogger())
	require.NoError(t, err)

	err = cm.Stop()
	assert.NoError(t, err)

	err = cm.Stop()
	assert.NoError(t, err)

	err = cm.Stop()
	assert.NoError(t, err)
}
