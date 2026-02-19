package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// ConfigMetrics tracks configuration-related metrics.
// Provides counters for config reload operations.
type ConfigMetrics struct {
	configReloadTotal *prometheus.CounterVec
}

// NewConfigMetrics creates and registers configuration metrics with the provided registry.
// Returns an error if metric registration fails.
func NewConfigMetrics(reg *prometheus.Registry) (*ConfigMetrics, error) {
	configReloadTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_config_reload_total",
			Help: "Total number of config reload attempts by success status",
		},
		[]string{"success"},
	)

	if err := registerCollector(reg, configReloadTotal); err != nil {
		return nil, fmt.Errorf("failed to register smaug_config_reload_total: %w", err)
	}

	return &ConfigMetrics{
		configReloadTotal: configReloadTotal,
	}, nil
}

// RecordConfigReload records a configuration reload attempt.
// success should be true if the reload succeeded, false otherwise.
// Does not return errors; metric recording is fail-safe.
func (c *ConfigMetrics) RecordConfigReload(success bool) {
	if c == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	successStr := successFalse
	if success {
		successStr = successTrue
	}
	c.configReloadTotal.WithLabelValues(successStr).Inc()
}
