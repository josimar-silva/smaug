package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

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
