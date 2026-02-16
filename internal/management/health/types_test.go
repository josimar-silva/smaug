package health_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/management/health"
)

func TestApplicationHealthMarshalsToJSON(t *testing.T) {
	// Given: an ApplicationHealth struct with all fields populated
	appHealth := health.ApplicationHealth{
		Status:            "healthy",
		Version:           "0.1.0-SNAPSHOT",
		ActiveRoutes:      3,
		ConfiguredServers: 5,
		Uptime:            "1h30m45s",
	}

	// When: marshalling to JSON
	data, err := json.Marshal(appHealth)

	// Then: JSON is valid and contains expected fields
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "healthy", result["status"])
	assert.Equal(t, "0.1.0-SNAPSHOT", result["version"])
	assert.Equal(t, float64(3), result["activeRoutes"])
	assert.Equal(t, float64(5), result["configuredServers"])
	assert.Equal(t, "1h30m45s", result["uptime"])
}

func TestLivenessResponseMarshalsToJSON(t *testing.T) {
	// Given: a LivenessResponse struct
	liveness := health.LivenessResponse{
		Status: "alive",
	}

	// When: marshalling to JSON
	data, err := json.Marshal(liveness)

	// Then: JSON is valid and contains status field
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "alive", result["status"])
}

func TestReadinessResponseMarshalsToJSON(t *testing.T) {
	tests := []struct {
		name     string
		response health.ReadinessResponse
		expected map[string]interface{}
	}{
		{
			name: "ready response with healthy status",
			response: health.ReadinessResponse{
				Status:       "ready",
				ActiveRoutes: 5,
			},
			expected: map[string]interface{}{
				"status":       "ready",
				"activeRoutes": float64(5),
			},
		},
		{
			name: "not ready response with zero routes",
			response: health.ReadinessResponse{
				Status:       "not ready",
				ActiveRoutes: 0,
			},
			expected: map[string]interface{}{
				"status":       "not ready",
				"activeRoutes": float64(0),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: marshalling to JSON
			data, err := json.Marshal(tt.response)

			// Then: JSON matches expected structure
			require.NoError(t, err)

			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expected["status"], result["status"])
			assert.Equal(t, tt.expected["activeRoutes"], result["activeRoutes"])
		})
	}
}

func TestVersionResponseMarshalsToJSON(t *testing.T) {
	// Given: a VersionResponse with build information
	versionResp := health.VersionResponse{
		Version:   "0.1.0-SNAPSHOT",
		BuildTime: "2026-02-16T12:00:00Z",
		GitCommit: "abc123def",
	}

	// When: marshalling to JSON
	data, err := json.Marshal(versionResp)

	// Then: JSON contains all version fields
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "0.1.0-SNAPSHOT", result["version"])
	assert.Equal(t, "2026-02-16T12:00:00Z", result["buildTime"])
	assert.Equal(t, "abc123def", result["gitCommit"])
}

func TestVersionInfoMarshalsToJSON(t *testing.T) {
	// Given: a VersionInfo with all fields
	versionInfo := health.VersionInfo{
		Version:   "0.1.0-SNAPSHOT",
		BuildTime: "2026-02-16T12:00:00Z",
		GitCommit: "abc123def",
	}

	// When: marshalling to JSON
	data, err := json.Marshal(versionInfo)

	// Then: JSON contains all fields with correct naming
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "0.1.0-SNAPSHOT", result["version"])
	assert.Equal(t, "2026-02-16T12:00:00Z", result["buildTime"])
	assert.Equal(t, "abc123def", result["gitCommit"])
}
