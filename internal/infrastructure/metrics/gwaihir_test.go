package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGwaihirMetricsRegistersCollectors(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()

	// When
	metrics, err := NewGwaihirMetrics(reg)
	require.NoError(t, err)

	// Then
	assert.NotNil(t, metrics)
}

func TestGwaihirMetricsRecordAPICallSuccess(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewGwaihirMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordAPICall("send_wol", true, 0.5)
	metrics.RecordAPICall("send_wol", false, 1.2)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	durationFound := false
	callsFound := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_gwaihir_api_duration_seconds" {
			durationFound = true
			assert.NotNil(t, mf.Metric)
		}
		if mf.GetName() == "smaug_gwaihir_api_calls_total" {
			callsFound = true
			assert.NotNil(t, mf.Metric)
		}
	}
	assert.True(t, durationFound, "smaug_gwaihir_api_duration_seconds metric not found")
	assert.True(t, callsFound, "smaug_gwaihir_api_calls_total metric not found")
}

func TestGwaihirMetricsRecordAPICallMultipleOperations(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewGwaihirMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordAPICall("send_wol", true, 0.5)
	metrics.RecordAPICall("check_status", false, 0.3)
	metrics.RecordAPICall("send_wol", true, 0.7)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	assert.NotNil(t, metricFamilies)
	assert.Greater(t, len(metricFamilies), 0)
}

func TestGwaihirMetricsRecordAPICallWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *GwaihirMetrics

	// When (should not panic)
	metrics.RecordAPICall("send_wol", true, 0.5)

	// Then (no error expected)
}
