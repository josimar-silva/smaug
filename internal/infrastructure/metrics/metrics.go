package metrics

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	successTrue  = "true"
	successFalse = "false"
)

var (
	// ErrMetricRegistrationFailed is returned when a metric fails to register with the Prometheus registry.
	ErrMetricRegistrationFailed = errors.New("metric registration failed")
)

type Registry struct {
	registry *prometheus.Registry
	Request  *RequestMetrics
	Power    *PowerMetrics
	Config   *ConfigMetrics
	Gwaihir  *GwaihirMetrics
}

// New creates a new Registry and initializes all metrics.
// Returns an error if metric registration fails.
func New() (*Registry, error) {
	reg := prometheus.NewRegistry()

	requestMetrics, err := NewRequestMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize request metrics: %w", err)
	}

	powerMetrics, err := NewPowerMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize power metrics: %w", err)
	}

	configMetrics, err := NewConfigMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config metrics: %w", err)
	}

	gwaihirMetrics, err := NewGwaihirMetrics(reg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gwaihir metrics: %w", err)
	}

	return &Registry{
		registry: reg,
		Request:  requestMetrics,
		Power:    powerMetrics,
		Config:   configMetrics,
		Gwaihir:  gwaihirMetrics,
	}, nil
}

// Handler returns an HTTP handler that exposes metrics in Prometheus format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// Gatherer returns the underlying prometheus.Gatherer for testing.
func (r *Registry) Gatherer() prometheus.Gatherer {
	return r.registry
}

// Collect implements prometheus.Collector interface for testing/debugging.
func (r *Registry) Collect(ch chan<- prometheus.Metric) {
	r.registry.Collect(ch)
}

// Describe implements prometheus.Collector interface for testing/debugging.
func (r *Registry) Describe(ch chan<- *prometheus.Desc) {
	r.registry.Describe(ch)
}

// Close cleans up resources (placeholder for future cleanup logic).
// Currently a no-op but provided for consistency with other infrastructure types.
func (r *Registry) Close() error {
	return nil
}

// registerCollector safely registers a collector with the registry.
// Returns an error if registration fails (e.g., collector already registered).
func registerCollector(reg *prometheus.Registry, collector prometheus.Collector) error {
	if err := reg.Register(collector); err != nil {
		return fmt.Errorf("failed to register collector: %w", err)
	}
	return nil
}
