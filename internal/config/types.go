package config

import "time"

// Config represents the root configuration structure for SMAUG.
// It defines all configuration options for the reverse proxy including
// settings, routes, and servers.
type Config struct {
	Settings SettingsConfig    `yaml:"settings"`
	Servers  map[string]Server `yaml:"servers"`
	Routes   []Route           `yaml:"routes"`
}

// SettingsConfig defines global settings for the application.
type SettingsConfig struct {
	Gwaihir       GwaihirConfig       `yaml:"gwaihir"`
	Logging       LoggingConfig       `yaml:"logging"`
	Observability ObservabilityConfig `yaml:"observability"`
}

// GwaihirConfig defines configuration for the Gwaihir WoL service.
type GwaihirConfig struct {
	URL     string        `yaml:"url"`
	APIKey  string        `yaml:"apiKey"`
	Timeout time.Duration `yaml:"timeout"`
}

// LoggingConfig defines logging configuration for structured logging.
type LoggingConfig struct {
	Level  string `yaml:"level"`  // info, debug, error
	Format string `yaml:"format"` // json or text
}

// ObservabilityConfig defines observability settings including health checks and metrics.
type ObservabilityConfig struct {
	HealthCheck HealthCheckConfig `yaml:"healthCheck"`
	Metrics     MetricsConfig     `yaml:"metrics"`
}

// HealthCheckConfig defines health check endpoint configuration.
type HealthCheckConfig struct {
	Enabled bool `yaml:"enabled"` // Enable /health, /live, /ready, /version endpoints
	Port    int  `yaml:"port"`    // Port for health check endpoints
}

// MetricsConfig defines metrics endpoint configuration.
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"` // Enable /metrics endpoint (Prometheus format)
	Port    int  `yaml:"port"`    // Port for metrics endpoint
}

// Server defines a physical or virtual machine that can be woken via Wake-on-LAN.
type Server struct {
	MAC         string            `yaml:"mac"`
	Broadcast   string            `yaml:"broadcast"`
	WakeOnLan   WakeOnLanConfig   `yaml:"wakeOnLan"`
	SleepOnLan  SleepOnLanConfig  `yaml:"sleepOnLan"`
	HealthCheck ServerHealthCheck `yaml:"healthCheck"`
}

// WakeOnLanConfig defines Wake-on-LAN configuration for a server.
type WakeOnLanConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Timeout  time.Duration `yaml:"timeout"`
	Debounce time.Duration `yaml:"debounce"` // Min interval between WoL attempts
}

// SleepOnLanConfig defines Sleep-on-LAN configuration for a server.
type SleepOnLanConfig struct {
	Enabled     bool          `yaml:"enabled"`
	Endpoint    string        `yaml:"endpoint"`
	AuthToken   string        `yaml:"authToken"`
	IdleTimeout time.Duration `yaml:"idleTimeout"` // Sleep after idle timeout
}

// ServerHealthCheck defines health check configuration for a server.
type ServerHealthCheck struct {
	Endpoint string        `yaml:"endpoint"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
}

// Route defines a single proxy route configuration (port-per-service).
type Route struct {
	Name     string `yaml:"name"`     // The route name
	Listen   int    `yaml:"listen"`   // The port to listen on
	Upstream string `yaml:"upstream"` // The target backend URL
	Server   string `yaml:"server"`   // The server name to associate with this route
}
