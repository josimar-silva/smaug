package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// PowerMetrics tracks power management metrics including WoL attempts, server state, and health checks.
// Provides counters for wake attempts and health failures,
// a gauge for server state, and a counter for sleep triggers.
type PowerMetrics struct {
	wakeAttemptsTotal   *prometheus.CounterVec
	serverAwake         *prometheus.GaugeVec
	healthCheckFailures *prometheus.CounterVec
	sleepTriggeredTotal *prometheus.CounterVec
}

// NewPowerMetrics creates and registers power management metrics with the provided registry.
// Returns an error if metric registration fails.
func NewPowerMetrics(reg *prometheus.Registry) (*PowerMetrics, error) {
	wakeAttemptsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_wake_attempts_total",
			Help: "Total number of WoL wake attempts by server and success status",
		},
		[]string{"server", "success"},
	)

	serverAwake := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "smaug_server_awake",
			Help: "Server state gauge (1=awake, 0=sleeping) by server",
		},
		[]string{"server"},
	)

	healthCheckFailures := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_health_check_failures_total",
			Help: "Total number of health check failures by server and reason",
		},
		[]string{"server", "reason"},
	)

	sleepTriggeredTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "smaug_sleep_triggered_total",
			Help: "Total number of servers put to sleep due to idle timeout",
		},
		[]string{"server"},
	)

	if err := registerCollector(reg, wakeAttemptsTotal); err != nil {
		return nil, fmt.Errorf("failed to register smaug_wake_attempts_total: %w", err)
	}
	if err := registerCollector(reg, serverAwake); err != nil {
		return nil, fmt.Errorf("failed to register smaug_server_awake: %w", err)
	}
	if err := registerCollector(reg, healthCheckFailures); err != nil {
		return nil, fmt.Errorf("failed to register smaug_health_check_failures_total: %w", err)
	}
	if err := registerCollector(reg, sleepTriggeredTotal); err != nil {
		return nil, fmt.Errorf("failed to register smaug_sleep_triggered_total: %w", err)
	}

	return &PowerMetrics{
		wakeAttemptsTotal:   wakeAttemptsTotal,
		serverAwake:         serverAwake,
		healthCheckFailures: healthCheckFailures,
		sleepTriggeredTotal: sleepTriggeredTotal,
	}, nil
}

// RecordWakeAttempt records a WoL wake attempt for a server.
// success should be true if the wake attempt succeeded, false otherwise.
// Does not return errors; metric recording is fail-safe.
func (p *PowerMetrics) RecordWakeAttempt(serverID string, success bool) {
	if p == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	successStr := successFalse
	if success {
		successStr = successTrue
	}
	p.wakeAttemptsTotal.WithLabelValues(serverID, successStr).Inc()
}

// SetServerAwake sets the server state gauge for a given server.
// awake should be true if the server is awake, false if sleeping.
// Does not return errors; metric recording is fail-safe.
func (p *PowerMetrics) SetServerAwake(serverID string, awake bool) {
	if p == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	value := 0.0
	if awake {
		value = 1.0
	}
	p.serverAwake.WithLabelValues(serverID).Set(value)
}

// RecordHealthCheckFailure records a health check failure for a server.
// reason should indicate the failure type (e.g., "timeout", "unhealthy_status", "network_error").
// Does not return errors; metric recording is fail-safe.
func (p *PowerMetrics) RecordHealthCheckFailure(serverID string, reason string) {
	if p == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	p.healthCheckFailures.WithLabelValues(serverID, reason).Inc()
}

// RecordSleepTriggered records that a server was put to sleep due to idle timeout.
// Does not return errors; metric recording is fail-safe.
func (p *PowerMetrics) RecordSleepTriggered(serverID string) {
	if p == nil {
		return
	}

	defer func() {
		_ = recover()
	}()

	p.sleepTriggeredTotal.WithLabelValues(serverID).Inc()
}
