package config

import (
	"log/slog"

	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the root configuration structure for SMAUG.
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
	APIKey  SecretString  `yaml:"apiKey"`
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
	AuthToken   SecretString  `yaml:"authToken"`
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

// SecretString is a string wrapper that redacts its value in logs, text
// marshalling, and string representations to prevent accidental exposure
// of sensitive configuration values such as API keys and auth tokens.
type SecretString struct {
	value string
}

// String returns a redacted placeholder if the value is set, or an empty
// string if unset. It implements the fmt.Stringer interface.
func (s SecretString) String() string {
	if s.value == "" {
		return ""
	}
	return "***REDACTED***"
}

// MarshalText returns the redacted representation, ensuring secrets are
// not leaked during text or JSON marshalling. It implements encoding.TextMarshaler.
func (s SecretString) MarshalText() ([]byte, error) {
	return []byte(s.String()), nil
}

// LogValue returns a redacted slog.Value for use with structured logging.
// It implements the slog.LogValuer interface.
func (s SecretString) LogValue() slog.Value {
	return slog.StringValue(s.String())
}

// Value returns the underlying plaintext secret. Use with care.
func (s SecretString) Value() string {
	return s.value
}

// UnmarshalYAML populates the SecretString from a YAML node.
func (s *SecretString) UnmarshalYAML(value *yaml.Node) error {
	return value.Decode(&s.value)
}
