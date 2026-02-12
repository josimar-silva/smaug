//go:build integration

package tests

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildSmaugBinary(t *testing.T) string {
	t.Helper()

	binaryPath := "/tmp/smaug-test"
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../cmd/smaug")
	buildOutput, err := buildCmd.CombinedOutput()

	require.NoError(t, err, "Failed to build application: %s", string(buildOutput))

	return binaryPath
}

func runBinaryWithTimeout(t *testing.T, binaryPath string, timeout time.Duration) string {
	t.Helper()

	runCmd := exec.Command(binaryPath)

	done := make(chan error, 1)
	var output []byte
	var err error

	go func() {
		output, err = runCmd.CombinedOutput()
		done <- err
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "Failed to run application. Output: %s", string(output))
	case <-time.After(timeout):
		if runCmd.Process != nil {
			_ = runCmd.Process.Kill()
		}
		require.Fail(t, "Application execution timed out after %v", timeout)
	}

	return strings.TrimSpace(string(output))
}

// TestIntegrationApplicationStartup verifies that the Smaug application
// can be built and executed successfully.
func TestIntegrationApplicationStartup(t *testing.T) {
	// Given: A compiled Smaug application binary
	binaryPath := buildSmaugBinary(t)

	// When: The application is executed
	actualOutput := runBinaryWithTimeout(t, binaryPath, 5*time.Second)

	// Then: The application outputs the expected startup message
	expectedOutput := "Smaug - Power Aware Reverse Proxy"
	assert.Equal(t, expectedOutput, actualOutput, "Application should output the expected startup message")
}
