package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ValidationErrors struct {
	errors []error
}

func (v *ValidationErrors) Add(err error) {
	if err != nil {
		v.errors = append(v.errors, err)
	}
}

func (v *ValidationErrors) Error() string {
	if len(v.errors) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range v.errors {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

func (v *ValidationErrors) HasErrors() bool {
	return len(v.errors) > 0
}

func (v *ValidationErrors) Unwrap() []error {
	return v.errors
}

// ErrorCount returns the number of errors collected.
func (v *ValidationErrors) ErrorCount() int {
	return len(v.errors)
}

var (
	macRegexColon  = regexp.MustCompile(`^([0-9A-Fa-f]{2}:){5}([0-9A-Fa-f]{2})$`)
	macRegexHyphen = regexp.MustCompile(`^([0-9A-Fa-f]{2}-){5}([0-9A-Fa-f]{2})$`)

	validLogLevels = map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	validLogFormats = map[string]bool{
		"json": true,
		"text": true,
	}
)

func (c *Config) Validate() error {
	errs := &ValidationErrors{}

	c.validateSettings(errs)
	c.validateServers(errs)
	c.validateRoutes(errs)

	if errs.HasErrors() {
		return errs
	}
	return nil
}

func (c *Config) validateSettings(errs *ValidationErrors) {
	c.validateGwaihir(errs)
	c.validateLogging(errs)
	c.validateObservability(errs)
}

func (c *Config) validateGwaihir(errs *ValidationErrors) {
	if c.Settings.Gwaihir.URL != "" {
		errs.Add(validateURL(c.Settings.Gwaihir.URL, "settings.gwaihir.url"))
	}
	if c.Settings.Gwaihir.Timeout != 0 {
		errs.Add(validateTimeout(c.Settings.Gwaihir.Timeout, "settings.gwaihir.timeout"))
	}
}

func (c *Config) validateLogging(errs *ValidationErrors) {
	if c.Settings.Logging.Level != "" {
		if !validLogLevels[c.Settings.Logging.Level] {
			errs.Add(fmt.Errorf("settings.logging.level: invalid level '%s', must be one of: debug, info, warn, error", c.Settings.Logging.Level))
		}
	}
	if c.Settings.Logging.Format != "" {
		if !validLogFormats[c.Settings.Logging.Format] {
			errs.Add(fmt.Errorf("settings.logging.format: invalid format '%s', must be one of: json, text", c.Settings.Logging.Format))
		}
	}
}

func (c *Config) validateObservability(errs *ValidationErrors) {
	if c.Settings.Observability.HealthCheck.Enabled {
		errs.Add(validatePort(c.Settings.Observability.HealthCheck.Port, "settings.observability.healthCheck.port"))
	}
	if c.Settings.Observability.Metrics.Enabled {
		errs.Add(validatePort(c.Settings.Observability.Metrics.Port, "settings.observability.metrics.port"))
	}
}

func (c *Config) validateServers(errs *ValidationErrors) {
	for name, server := range c.Servers {
		errs.Add(validateMAC(server.MAC, fmt.Sprintf("servers.%s.mac", name)))
		errs.Add(validateIP(server.Broadcast, fmt.Sprintf("servers.%s.broadcast", name)))
		server.validateWakeOnLan(errs, name)
		server.validateSleepOnLan(errs, name)
		server.validateHealthCheck(errs, name)
	}
}

func (server *Server) validateWakeOnLan(errs *ValidationErrors, name string) {
	if !server.WakeOnLan.Enabled {
		return
	}

	if server.WakeOnLan.Timeout != 0 {
		errs.Add(validateTimeout(server.WakeOnLan.Timeout, fmt.Sprintf("servers.%s.wakeOnLan.timeout", name)))
	}

	if server.WakeOnLan.Debounce != 0 {
		errs.Add(validateTimeout(server.WakeOnLan.Debounce, fmt.Sprintf("servers.%s.wakeOnLan.debounce", name)))
	}
}

func (server *Server) validateSleepOnLan(errs *ValidationErrors, name string) {
	if !server.SleepOnLan.Enabled {
		return
	}

	errs.Add(validateURL(server.SleepOnLan.Endpoint, fmt.Sprintf("servers.%s.sleepOnLan.endpoint", name)))
	if server.SleepOnLan.IdleTimeout != 0 {
		errs.Add(validateTimeout(server.SleepOnLan.IdleTimeout, fmt.Sprintf("servers.%s.sleepOnLan.idleTimeout", name)))
	}
}

func (server *Server) validateHealthCheck(errs *ValidationErrors, name string) {
	if server.HealthCheck.Endpoint == "" {
		return
	}

	errs.Add(validateURL(server.HealthCheck.Endpoint, fmt.Sprintf("servers.%s.healthCheck.endpoint", name)))

	if server.HealthCheck.Interval != 0 {
		errs.Add(validateTimeout(server.HealthCheck.Interval, fmt.Sprintf("servers.%s.healthCheck.interval", name)))
	}

	if server.HealthCheck.Timeout != 0 {
		errs.Add(validateTimeout(server.HealthCheck.Timeout, fmt.Sprintf("servers.%s.healthCheck.timeout", name)))
	}
}

func (c *Config) validateRoutes(errs *ValidationErrors) {
	for i, route := range c.Routes {
		if route.Name == "" {
			errs.Add(fmt.Errorf("routes[%d].name: cannot be empty", i))
		}
		errs.Add(validatePort(route.Listen, fmt.Sprintf("routes[%d].listen", i)))
		errs.Add(validateURL(route.Upstream, fmt.Sprintf("routes[%d].upstream", i)))

		if route.Server != "" {
			if _, exists := c.Servers[route.Server]; !exists {
				errs.Add(fmt.Errorf("routes[%d].server: references non-existent server '%s'", i, route.Server))
			}
		}
	}
}

func validateMAC(mac, field string) error {
	if mac == "" {
		return fmt.Errorf("%s: MAC address cannot be empty", field)
	}
	if !macRegexColon.MatchString(mac) && !macRegexHyphen.MatchString(mac) {
		return fmt.Errorf("%s: invalid MAC address format '%s'", field, mac)
	}
	return nil
}

func validateIP(ip, field string) error {
	if ip == "" {
		return fmt.Errorf("%s: IP address cannot be empty", field)
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return fmt.Errorf("%s: invalid IP address '%s'", field, ip)
	}
	return nil
}

func validateURL(rawURL, field string) error {
	if rawURL == "" {
		return fmt.Errorf("%s: URL cannot be empty", field)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s: invalid URL '%s': %w", field, rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s: URL scheme must be http or https, got '%s'", field, parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s: URL must have a host", field)
	}
	return nil
}

func validatePort(port int, field string) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s: port must be between 1 and 65535, got %d", field, port)
	}
	return nil
}

func validateTimeout(timeout time.Duration, field string) error {
	if timeout <= 0 {
		return fmt.Errorf("%s: timeout must be greater than 0, got %s", field, timeout)
	}
	return nil
}

func IsValidationError(err error) bool {
	var validationErr *ValidationErrors
	return errors.As(err, &validationErr)
}
