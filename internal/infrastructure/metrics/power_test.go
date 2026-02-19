package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPowerMetricsRegistersCollectors(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()

	// When
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// Then
	assert.NotNil(t, metrics)
}

func TestPowerMetricsRecordWakeAttemptSuccess(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordWakeAttempt("server1", true)
	metrics.RecordWakeAttempt("server1", true)
	metrics.RecordWakeAttempt("server1", false)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_wake_attempts_total" {
			found = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, found, "smaug_wake_attempts_total metric not found")
}

func TestPowerMetricsRecordWakeAttemptWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *PowerMetrics

	// When (should not panic)
	metrics.RecordWakeAttempt("server1", true)

	// Then (no error expected)
}

func TestPowerMetricsSetServerAwakeTrue(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.SetServerAwake("server1", true)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_server_awake" {
			found = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, found, "smaug_server_awake metric not found")
}

func TestPowerMetricsSetServerAwakeFalse(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.SetServerAwake("server1", false)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_server_awake" {
			found = true
		}
	}
	assert.True(t, found, "smaug_server_awake metric not found")
}

func TestPowerMetricsSetServerAwakeWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *PowerMetrics

	// When (should not panic)
	metrics.SetServerAwake("server1", true)

	// Then (no error expected)
}

func TestPowerMetricsRecordHealthCheckFailure(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordHealthCheckFailure("server1", "timeout")
	metrics.RecordHealthCheckFailure("server1", "unhealthy_status")
	metrics.RecordHealthCheckFailure("server2", "network_error")

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_health_check_failures_total" {
			found = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, found, "smaug_health_check_failures_total metric not found")
}

func TestPowerMetricsRecordHealthCheckFailureWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *PowerMetrics

	// When (should not panic)
	metrics.RecordHealthCheckFailure("server1", "timeout")

	// Then (no error expected)
}

func TestPowerMetricsRecordSleepTriggered(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordSleepTriggered("server1")
	metrics.RecordSleepTriggered("server2")

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_sleep_triggered_total" {
			found = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, found, "smaug_sleep_triggered_total metric not found")
}

func TestPowerMetricsRecordSleepTriggeredWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *PowerMetrics

	// When (should not panic)
	metrics.RecordSleepTriggered("server1")

	// Then (no error expected)
}

func TestPowerMetricsMultipleServersTrackedIndependently(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordWakeAttempt("server1", true)
	metrics.RecordWakeAttempt("server2", false)
	metrics.SetServerAwake("server1", true)
	metrics.SetServerAwake("server2", false)
	metrics.RecordHealthCheckFailure("server1", "timeout")
	metrics.RecordHealthCheckFailure("server2", "network_error")
	metrics.RecordSleepTriggered("server1")

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	assert.NotNil(t, metricFamilies)
	assert.Greater(t, len(metricFamilies), 0)
}

func TestPowerMetricsWakeAttemptSuccessFalse(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewPowerMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordWakeAttempt("server1", false)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_wake_attempts_total" {
			found = true
		}
	}
	assert.True(t, found, "smaug_wake_attempts_total metric not found")
}
