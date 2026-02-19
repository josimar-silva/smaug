package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// GwaihirMetrics tracks Gwaihir API interaction metrics.
// Provides histograms for API call duration and counters for operation outcomes.
type GwaihirMetrics struct {
	apiDurationSeconds *prometheus.HistogramVec
	apiCallsTotal      *prometheus.CounterVec
}

// NewGwaihirMetrics creates and registers Gwaihir API metrics with the provided registry.
// Returns an error if metric registration fails.
func NewGwaihirMetrics(reg *prometheus.Registry) (*GwaihirMetrics, error) {
	apiDurationSeconds := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "smaug_gwaihir_api_duration_seconds",
			Help:    "Gwaihir API call duration in seconds by operation and success status",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "success"},
	)

	apiCallsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_gwaihir_api_calls_total",
			Help: "Total number of Gwaihir API calls by operation and success status",
		},
		[]string{"operation", "success"},
	)

	if err := registerCollector(reg, apiDurationSeconds); err != nil {
		return nil, fmt.Errorf("failed to register smaug_gwaihir_api_duration_seconds: %w", err)
	}
	if err := registerCollector(reg, apiCallsTotal); err != nil {
		return nil, fmt.Errorf("failed to register smaug_gwaihir_api_calls_total: %w", err)
	}

	return &GwaihirMetrics{
		apiDurationSeconds: apiDurationSeconds,
		apiCallsTotal:      apiCallsTotal,
	}, nil
}

// RecordAPICall records a Gwaihir API call with its duration.
// operation should be the API operation name (e.g., "send_wol", "check_status").
// success should be true if the call succeeded, false otherwise.
// durationSeconds is the API call duration in seconds.
// Does not return errors; metric recording is fail-safe.
func (g *GwaihirMetrics) RecordAPICall(operation string, success bool, durationSeconds float64) {
	if g == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	successStr := successFalse
	if success {
		successStr = successTrue
	}

	g.apiDurationSeconds.WithLabelValues(operation, successStr).Observe(durationSeconds)
	g.apiCallsTotal.WithLabelValues(operation, successStr).Inc()
}
