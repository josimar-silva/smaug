package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type stringValidationTest struct {
	name      string
	value     string
	field     string
	wantError bool
}

func testStringValidation(t *testing.T, fnName string, fn func(string, string) error, tests []stringValidationTest) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fn(tt.value, tt.field)
			if (err != nil) != tt.wantError {
				t.Errorf("%s() error = %v, wantError %v", fnName, err, tt.wantError)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	testStringValidation(t, "validateURL", validateURL, []stringValidationTest{
		{"valid HTTP URL", "http://example.com", "test.url", false},
		{"valid HTTPS URL", "https://example.com", "test.url", false},
		{"valid URL with port", "http://example.com:8080", "test.url", false},
		{"valid URL with path", "https://example.com/api/v1", "test.url", false},
		{"valid URL with query", "https://example.com?key=value", "test.url", false},
		{"valid URL with fragment", "https://example.com#section", "test.url", false},
		{"valid URL localhost", "http://localhost:3000", "test.url", false},
		{"valid URL IP address", "http://192.168.1.1:8080", "test.url", false},
		{"valid URL with auth", "https://user:pass@example.com", "test.url", false},
		{"empty URL", "", "test.url", true},
		{"invalid scheme FTP", "ftp://example.com", "test.url", true},
		{"invalid scheme WS", "ws://example.com", "test.url", true},
		{"invalid no scheme", "example.com", "test.url", true},
		{"invalid no host", "http://", "test.url", true},
		{"invalid malformed", "http://[invalid", "test.url", true},
		{"invalid just path", "/api/v1", "test.url", true},
	})
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		field     string
		wantError bool
	}{
		{"valid port 80", 80, "test.port", false},
		{"valid port 443", 443, "test.port", false},
		{"valid port 8080", 8080, "test.port", false},
		{"valid port minimum 1", 1, "test.port", false},
		{"valid port maximum 65535", 65535, "test.port", false},
		{"valid port 3000", 3000, "test.port", false},
		{"valid port 5432", 5432, "test.port", false},
		{"valid port 27017", 27017, "test.port", false},
		{"invalid port 0", 0, "test.port", true},
		{"invalid port negative", -1, "test.port", true},
		{"invalid port too large", 65536, "test.port", true},
		{"invalid port way too large", 100000, "test.port", true},
		{"invalid port -8080", -8080, "test.port", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePort(tt.port, tt.field)
			if (err != nil) != tt.wantError {
				t.Errorf("validatePort() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		field     string
		wantError bool
	}{
		{"valid timeout 1s", 1 * time.Second, "test.timeout", false},
		{"valid timeout 30s", 30 * time.Second, "test.timeout", false},
		{"valid timeout 1m", 1 * time.Minute, "test.timeout", false},
		{"valid timeout 5m", 5 * time.Minute, "test.timeout", false},
		{"valid timeout 1h", 1 * time.Hour, "test.timeout", false},
		{"valid timeout 100ms", 100 * time.Millisecond, "test.timeout", false},
		{"valid timeout 1ns", 1 * time.Nanosecond, "test.timeout", false},
		{"valid timeout very large", 24 * time.Hour, "test.timeout", false},
		{"invalid timeout 0", 0, "test.timeout", true},
		{"invalid timeout negative 1s", -1 * time.Second, "test.timeout", true},
		{"invalid timeout negative 5m", -5 * time.Minute, "test.timeout", true},
		{"invalid timeout negative 1ns", -1 * time.Nanosecond, "test.timeout", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTimeout(tt.timeout, tt.field)
			if (err != nil) != tt.wantError {
				t.Errorf("validateTimeout() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestConfigValidateMultipleErrors(t *testing.T) {
	config := &Config{
		Settings: SettingsConfig{
			Gwaihir: GwaihirConfig{
				URL:     "invalid-url",
				Timeout: -1 * time.Second,
			},
			Observability: ObservabilityConfig{
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Port:    0,
				},
				Metrics: MetricsConfig{
					Enabled: true,
					Port:    99999,
				},
			},
		},
		Servers: map[string]Server{
			"server1": {},
		},
		Routes: []Route{
			{
				Name:     "",
				Listen:   -1,
				Upstream: "not-a-url",
			},
		},
	}

	err := config.Validate()
	if err == nil {
		t.Fatal("expected validation errors, got nil")
	}

	var validationErr *ValidationErrors
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationErrors, got %T", err)
	}

	if validationErr.ErrorCount() < 7 {
		t.Errorf("expected at least 7 errors, got %d", validationErr.ErrorCount())
	}

	errMsg := err.Error()
	expectedSubstrings := []string{
		"settings.gwaihir.url",
		"settings.gwaihir.timeout",
		"settings.observability.healthCheck.port",
		"settings.observability.metrics.port",
		"routes[0].name",
		"routes[0].listen",
		"routes[0].upstream",
	}

	for _, substring := range expectedSubstrings {
		if !strings.Contains(errMsg, substring) {
			t.Errorf("error message should contain '%s', got: %s", substring, errMsg)
		}
	}
}

func TestConfigValidateValidConfig(t *testing.T) {
	config := &Config{
		Settings: SettingsConfig{
			Gwaihir: GwaihirConfig{
				URL:     "https://gwaihir.example.com",
				Timeout: 30 * time.Second,
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
			Observability: ObservabilityConfig{
				HealthCheck: HealthCheckConfig{
					Enabled: true,
					Port:    8080,
				},
				Metrics: MetricsConfig{
					Enabled: true,
					Port:    9090,
				},
			},
		},
		Servers: map[string]Server{
			"server1": {
				WakeOnLan: WakeOnLanConfig{
					Enabled:  true,
					Timeout:  1 * time.Minute,
					Debounce: 30 * time.Second,
				},
				SleepOnLan: SleepOnLanConfig{
					Enabled:     true,
					Endpoint:    "http://192.168.1.100:3000/sleep",
					IdleTimeout: 1 * time.Hour,
				},
				HealthCheck: ServerHealthCheck{
					Endpoint: "http://192.168.1.100:8080/health",
					Interval: 30 * time.Second,
					Timeout:  5 * time.Second,
				},
			},
		},
		Routes: []Route{
			{
				Name:     "web",
				Listen:   80,
				Upstream: "http://192.168.1.100:8080",
				Server:   "server1",
			},
		},
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("expected no validation errors for valid config, got: %v", err)
	}
}

func TestConfigValidateLoggingLevels(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantError bool
	}{
		{"valid level debug", "debug", false},
		{"valid level info", "info", false},
		{"valid level warn", "warn", false},
		{"valid level error", "error", false},
		{"invalid level trace", "trace", true},
		{"invalid level fatal", "fatal", true},
		{"invalid level DEBUG uppercase", "DEBUG", true},
		{"empty level", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Settings: SettingsConfig{
					Logging: LoggingConfig{
						Level: tt.level,
					},
				},
			}

			err := config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() with level=%s error = %v, wantError %v", tt.level, err, tt.wantError)
			}
		})
	}
}

func TestConfigValidateLoggingFormats(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		wantError bool
	}{
		{"valid format json", "json", false},
		{"valid format text", "text", false},
		{"invalid format xml", "xml", true},
		{"invalid format yaml", "yaml", true},
		{"invalid format JSON uppercase", "JSON", true},
		{"empty format", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Settings: SettingsConfig{
					Logging: LoggingConfig{
						Format: tt.format,
					},
				},
			}

			err := config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() with format=%s error = %v, wantError %v", tt.format, err, tt.wantError)
			}
		})
	}
}

func TestConfigValidateRouteServerReference(t *testing.T) {
	tests := []struct {
		name      string
		serverRef string
		wantError bool
	}{
		{"valid server reference", "server1", false},
		{"empty server reference", "", false},
		{"non-existent server reference", "nonexistent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Servers: map[string]Server{
					"server1": {},
				},
				Routes: []Route{
					{
						Name:     "test",
						Listen:   8080,
						Upstream: "http://example.com",
						Server:   tt.serverRef,
					},
				},
			}

			err := config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() with serverRef=%s error = %v, wantError %v", tt.serverRef, err, tt.wantError)
			}
		})
	}
}

func TestConfigValidateConditionalValidation(t *testing.T) {
	t.Run("disabled health check skips port validation", func(t *testing.T) {
		config := &Config{
			Settings: SettingsConfig{
				Observability: ObservabilityConfig{
					HealthCheck: HealthCheckConfig{
						Enabled: false,
						Port:    0,
					},
				},
			},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("expected no error for disabled health check with port 0, got: %v", err)
		}
	})

	t.Run("disabled metrics skips port validation", func(t *testing.T) {
		config := &Config{
			Settings: SettingsConfig{
				Observability: ObservabilityConfig{
					Metrics: MetricsConfig{
						Enabled: false,
						Port:    99999,
					},
				},
			},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("expected no error for disabled metrics with invalid port, got: %v", err)
		}
	})

	t.Run("disabled WoL skips timeout validation", func(t *testing.T) {
		config := &Config{
			Servers: map[string]Server{
				"server1": {
					WakeOnLan: WakeOnLanConfig{
						Enabled: false,
						Timeout: -1 * time.Second,
					},
				},
			},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("expected no error for disabled WoL with invalid timeout, got: %v", err)
		}
	})

	t.Run("disabled SleepOnLan skips endpoint validation", func(t *testing.T) {
		config := &Config{
			Servers: map[string]Server{
				"server1": {
					SleepOnLan: SleepOnLanConfig{
						Enabled:  false,
						Endpoint: "invalid-url",
					},
				},
			},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("expected no error for disabled SleepOnLan with invalid endpoint, got: %v", err)
		}
	})

	t.Run("empty health check endpoint skips validation", func(t *testing.T) {
		config := &Config{
			Servers: map[string]Server{
				"server1": {
					HealthCheck: ServerHealthCheck{
						Endpoint: "",
						Interval: 0,
						Timeout:  0,
					},
				},
			},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("expected no error for empty health check endpoint, got: %v", err)
		}
	})
}

func TestIsValidationError(t *testing.T) {
	t.Run("returns true for ValidationErrors", func(t *testing.T) {
		err := &ValidationErrors{}
		err.Add(errors.New("test error"))

		if !IsValidationError(err) {
			t.Error("expected IsValidationError to return true for ValidationErrors")
		}
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		err := errors.New("regular error")

		if IsValidationError(err) {
			t.Error("expected IsValidationError to return false for regular error")
		}
	})

	t.Run("returns false for nil", func(t *testing.T) {
		if IsValidationError(nil) {
			t.Error("expected IsValidationError to return false for nil")
		}
	})
}

func TestValidationErrorsUnwrap(t *testing.T) {
	errs := &ValidationErrors{}
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	err3 := errors.New("error 3")

	errs.Add(err1)
	errs.Add(err2)
	errs.Add(err3)

	unwrapped := errs.Unwrap()
	if len(unwrapped) != 3 {
		t.Errorf("expected 3 unwrapped errors, got %d", len(unwrapped))
	}

	if !errors.Is(errs, err1) || !errors.Is(errs, err2) || !errors.Is(errs, err3) {
		t.Error("expected all errors to be accessible via errors.Is")
	}
}

func TestValidationErrorsErrorCount(t *testing.T) {
	tests := []struct {
		name      string
		numErrors int
	}{
		{"no errors", 0},
		{"single error", 1},
		{"multiple errors", 5},
		{"many errors", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := &ValidationErrors{}
			for i := 0; i < tt.numErrors; i++ {
				errs.Add(errors.New("test error"))
			}

			if errs.ErrorCount() != tt.numErrors {
				t.Errorf("ErrorCount() = %d, want %d", errs.ErrorCount(), tt.numErrors)
			}
		})
	}
}

func TestValidateBasicAuthToken(t *testing.T) {
	testStringValidation(t, "validateBasicAuthToken", validateBasicAuthToken, []stringValidationTest{
		// Valid: base64(user:password)
		{"valid user:password", "dXNlcjpwYXNzd29yZA==", "test.authToken", false},
		// Valid: base64(admin:secret) — different credentials
		{"valid admin:secret", "YWRtaW46c2VjcmV0", "test.authToken", false},
		// Valid: base64(user:) — empty password is allowed
		{"valid user with empty password", "dXNlcjo=", "test.authToken", false},
		// Invalid: not valid base64 at all
		{"invalid base64", "not-valid-base64!!!", "test.authToken", true},
		// Invalid: valid base64 but decodes to string with no colon (no user:password separator)
		{"valid base64 but no colon", "aGVsbG93b3JsZA==", "test.authToken", true}, // base64("helloworld")
		// Invalid: valid base64 but decodes to :password (empty username)
		{"valid base64 but empty username", "OnBhc3N3b3Jk", "test.authToken", true}, // base64(":password")
	})
}

func TestConfigValidateHealthCheckAuthToken(t *testing.T) {
	makeConfig := func(token string) *Config {
		return &Config{
			Servers: map[string]Server{
				"server1": {
					HealthCheck: ServerHealthCheck{
						Endpoint:  "http://192.168.1.100:8080/health",
						AuthToken: SecretString{value: token},
					},
				},
			},
		}
	}

	t.Run("valid base64 user:password passes validation", func(t *testing.T) {
		cfg := makeConfig("dXNlcjpwYXNzd29yZA==") // base64("user:password")
		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected no validation error for valid authToken, got: %v", err)
		}
	})

	t.Run("invalid base64 string fails validation", func(t *testing.T) {
		cfg := makeConfig("not-valid-base64!!!")
		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected validation error for invalid base64, got nil")
		}
		if !strings.Contains(err.Error(), "servers.server1.healthCheck.authToken") {
			t.Errorf("error should reference the authToken field path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "invalid base64 encoding") {
			t.Errorf("error should mention invalid base64, got: %v", err)
		}
	})

	t.Run("valid base64 without colon fails validation", func(t *testing.T) {
		cfg := makeConfig("aGVsbG93b3JsZA==") // base64("helloworld"), no colon
		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected validation error for base64 without colon, got nil")
		}
		if !strings.Contains(err.Error(), "servers.server1.healthCheck.authToken") {
			t.Errorf("error should reference the authToken field path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "user:password") {
			t.Errorf("error should mention expected format, got: %v", err)
		}
	})

	t.Run("empty auth token skips validation", func(t *testing.T) {
		cfg := makeConfig("")
		err := cfg.Validate()
		if err != nil {
			t.Errorf("expected no validation error for empty authToken, got: %v", err)
		}
	})
}
