package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigMetricsRegistersCollectors(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()

	// When
	metrics, err := NewConfigMetrics(reg)
	require.NoError(t, err)

	// Then
	assert.NotNil(t, metrics)
}

func TestConfigMetricsRecordConfigReloadSuccess(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewConfigMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordConfigReload(true)
	metrics.RecordConfigReload(true)
	metrics.RecordConfigReload(false)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_config_reload_total" {
			found = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, found, "smaug_config_reload_total metric not found")
}

func TestConfigMetricsRecordConfigReloadWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *ConfigMetrics

	// When (should not panic)
	metrics.RecordConfigReload(true)

	// Then (no error expected)
}
