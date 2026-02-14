package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
