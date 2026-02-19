package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// RequestMetrics tracks HTTP request metrics.
// Provides counters for request counts by method/path/status,
// and histograms for request duration by method/path.
type RequestMetrics struct {
	requestsTotal    prometheus.Counter
	requestDuration  prometheus.Histogram
	requestsByStatus *prometheus.CounterVec
	requestsByMethod *prometheus.CounterVec
}

// NewRequestMetrics creates and registers request metrics with the provided registry.
// Returns an error if metric registration fails.
func NewRequestMetrics(reg *prometheus.Registry) (*RequestMetrics, error) {
	requestsTotal := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "smaug_requests_total",
			Help: "Total number of HTTP requests processed",
		},
	)

	requestDuration := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "smaug_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	requestsByStatus := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_requests_by_status_total",
			Help: "Total number of HTTP requests by status code",
		},
		[]string{"status"},
	)

	requestsByMethod := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_requests_by_method_total",
			Help: "Total number of HTTP requests by method",
		},
		[]string{"method"},
	)

	if err := registerCollector(reg, requestsTotal); err != nil {
		return nil, fmt.Errorf("failed to register smaug_requests_total: %w", err)
	}
	if err := registerCollector(reg, requestDuration); err != nil {
		return nil, fmt.Errorf("failed to register smaug_request_duration_seconds: %w", err)
	}
	if err := registerCollector(reg, requestsByStatus); err != nil {
		return nil, fmt.Errorf("failed to register smaug_requests_by_status_total: %w", err)
	}
	if err := registerCollector(reg, requestsByMethod); err != nil {
		return nil, fmt.Errorf("failed to register smaug_requests_by_method_total: %w", err)
	}

	return &RequestMetrics{
		requestsTotal:    requestsTotal,
		requestDuration:  requestDuration,
		requestsByStatus: requestsByStatus,
		requestsByMethod: requestsByMethod,
	}, nil
}

// RecordRequest records a completed HTTP request.
// Should be called for each HTTP request processed, with:
//   - method: HTTP method (GET, POST, etc.)
//   - status: HTTP response status code (200, 404, 500, etc.)
//   - durationSeconds: request duration in seconds
//
// Does not return errors; metric recording is fail-safe.
func (r *RequestMetrics) RecordRequest(method string, statusCode int, durationSeconds float64) {
	if r == nil {
		return
	}

	// Safely record metrics (no panics even if something goes wrong)
	defer func() {
		_ = recover()
	}()

	r.requestsTotal.Inc()
	r.requestDuration.Observe(durationSeconds)
	r.requestsByStatus.WithLabelValues(statusCodeToString(statusCode)).Inc()
	r.requestsByMethod.WithLabelValues(method).Inc()
}

// statusCodeToString converts a numeric HTTP status code to its string representation.
func statusCodeToString(code int) string {
	switch code {
	case 200:
		return "200"
	case 201:
		return "201"
	case 204:
		return "204"
	case 301:
		return "301"
	case 302:
		return "302"
	case 304:
		return "304"
	case 400:
		return "400"
	case 401:
		return "401"
	case 403:
		return "403"
	case 404:
		return "404"
	case 409:
		return "409"
	case 429:
		return "429"
	case 500:
		return "500"
	case 502:
		return "502"
	case 503:
		return "503"
	case 504:
		return "504"
	default:
		return "other"
	}
}
