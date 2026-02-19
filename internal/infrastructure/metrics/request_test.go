package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRequestMetricsRegistersCollectors(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()

	// When
	metrics, err := NewRequestMetrics(reg)

	// Then
	require.NoError(t, err)
	require.NotNil(t, metrics)
	assert.NotNil(t, metrics.requestsTotal)
	assert.NotNil(t, metrics.requestDuration)
	assert.NotNil(t, metrics.requestsByStatus)
	assert.NotNil(t, metrics.requestsByMethod)
}

func TestRequestMetricsRecordRequestIncrementsCounter(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewRequestMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordRequest("GET", 200, 0.5)
	metrics.RecordRequest("POST", 201, 1.2)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	var requestsTotalFound bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_requests_total" {
			requestsTotalFound = true
			assert.NotNil(t, mf.Metric)
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, requestsTotalFound, "smaug_requests_total metric not found")
}

func TestRequestMetricsRecordRequestRecordsHistogram(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewRequestMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordRequest("GET", 200, 0.5)
	metrics.RecordRequest("GET", 200, 1.5)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	var histogramFound bool
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_request_duration_seconds" {
			histogramFound = true
			assert.NotNil(t, mf.Metric)
		}
	}
	assert.True(t, histogramFound, "smaug_request_duration_seconds metric not found")
}

func TestRequestMetricsRecordRequestRecordsByStatus(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewRequestMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordRequest("GET", 200, 0.1)
	metrics.RecordRequest("GET", 404, 0.2)
	metrics.RecordRequest("POST", 500, 0.3)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	statusMetricsFound := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_requests_by_status_total" {
			statusMetricsFound = true
			assert.NotNil(t, mf.Metric)
			// Should have metrics for 200, 404, 500
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, statusMetricsFound, "smaug_requests_by_status_total metric not found")
}

func TestRequestMetricsRecordRequestRecordsByMethod(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewRequestMetrics(reg)
	require.NoError(t, err)

	// When
	metrics.RecordRequest("GET", 200, 0.1)
	metrics.RecordRequest("POST", 200, 0.2)
	metrics.RecordRequest("PUT", 200, 0.3)

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)

	methodMetricsFound := false
	for _, mf := range metricFamilies {
		if mf.GetName() == "smaug_requests_by_method_total" {
			methodMetricsFound = true
			assert.NotNil(t, mf.Metric)
			// Should have metrics for GET, POST, PUT
			assert.Greater(t, len(mf.Metric), 0)
		}
	}
	assert.True(t, methodMetricsFound, "smaug_requests_by_method_total metric not found")
}

func TestRequestMetricsRecordRequestWithNilMetricsSafe(t *testing.T) {
	// Given
	var metrics *RequestMetrics

	// When (should not panic)
	metrics.RecordRequest("GET", 200, 0.5)

	// Then (no error expected)
}

func TestStatusCodeToStringMapsCommonStatuses(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{200, "200"},
		{201, "201"},
		{204, "204"},
		{301, "301"},
		{302, "302"},
		{304, "304"},
		{400, "400"},
		{401, "401"},
		{403, "403"},
		{404, "404"},
		{409, "409"},
		{429, "429"},
		{500, "500"},
		{502, "502"},
		{503, "503"},
		{504, "504"},
		{418, "other"}, // Unknown status
		{999, "other"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			// When
			result := statusCodeToString(tt.code)

			// Then
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRequestMetricsRecordRequestMultipleCalls(t *testing.T) {
	// Given
	reg := prometheus.NewRegistry()
	metrics, err := NewRequestMetrics(reg)
	require.NoError(t, err)

	// When
	for i := 0; i < 10; i++ {
		metrics.RecordRequest("GET", 200, 0.1)
	}
	for i := 0; i < 5; i++ {
		metrics.RecordRequest("POST", 201, 0.2)
	}

	// Then
	metricFamilies, err := reg.Gather()
	require.NoError(t, err)
	assert.NotNil(t, metricFamilies)
	assert.Greater(t, len(metricFamilies), 0)
}
