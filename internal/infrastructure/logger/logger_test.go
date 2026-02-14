package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name       string
		level      logger.Level
		logHandler logger.LogHandler
		output     *bytes.Buffer
	}{
		{
			name:       "creates logger with debug level and JSON handler",
			level:      logger.LevelDebug,
			logHandler: logger.JSON,
			output:     &bytes.Buffer{},
		},
		{
			name:       "creates logger with info level and JSON handler",
			level:      logger.LevelInfo,
			logHandler: logger.JSON,
			output:     &bytes.Buffer{},
		},
		{
			name:       "creates logger with warn level and text handler",
			level:      logger.LevelWarn,
			logHandler: logger.TEXT,
			output:     &bytes.Buffer{},
		},
		{
			name:       "creates logger with error level and text handler",
			level:      logger.LevelError,
			logHandler: logger.TEXT,
			output:     &bytes.Buffer{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logger.New(tt.level, tt.logHandler, tt.output)
			assert.NotNil(t, log)
		})
	}
}

func TestNewNilOutput(t *testing.T) {
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	assert.NotNil(t, log)
}

func TestJSONFormat(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*logger.Logger, *bytes.Buffer)
		expected map[string]any
	}{
		{
			name: "debug log produces valid JSON",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Debug("test message", "key", "value")
			},
			expected: map[string]any{
				"level": "DEBUG",
				"msg":   "test message",
				"key":   "value",
			},
		},
		{
			name: "info log produces valid JSON",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Info("test message", "key", "value")
			},
			expected: map[string]any{
				"level": "INFO",
				"msg":   "test message",
				"key":   "value",
			},
		},
		{
			name: "warn log produces valid JSON",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Warn("test message", "key", "value")
			},
			expected: map[string]any{
				"level": "WARN",
				"msg":   "test message",
				"key":   "value",
			},
		},
		{
			name: "error log produces valid JSON",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Error("test message", "key", "value")
			},
			expected: map[string]any{
				"level": "ERROR",
				"msg":   "test message",
				"key":   "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelDebug, logger.JSON, buf)

			tt.logFunc(log, buf)

			var result map[string]any
			err := json.Unmarshal(buf.Bytes(), &result)
			require.NoError(t, err, "output should be valid JSON")

			assert.Equal(t, tt.expected["level"], result["level"])
			assert.Equal(t, tt.expected["msg"], result["msg"])
			assert.Equal(t, tt.expected["key"], result["key"])
			assert.NotEmpty(t, result["time"], "timestamp should be present")
		})
	}
}

func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name        string
		level       logger.Level
		logFunc     func(*logger.Logger)
		shouldLog   bool
		description string
	}{
		{
			name:        "debug level logs debug messages",
			level:       logger.LevelDebug,
			logFunc:     func(l *logger.Logger) { l.Debug("debug message") },
			shouldLog:   true,
			description: "debug level should include debug messages",
		},
		{
			name:        "info level filters debug messages",
			level:       logger.LevelInfo,
			logFunc:     func(l *logger.Logger) { l.Debug("debug message") },
			shouldLog:   false,
			description: "info level should filter debug messages",
		},
		{
			name:        "info level logs info messages",
			level:       logger.LevelInfo,
			logFunc:     func(l *logger.Logger) { l.Info("info message") },
			shouldLog:   true,
			description: "info level should include info messages",
		},
		{
			name:        "warn level filters info messages",
			level:       logger.LevelWarn,
			logFunc:     func(l *logger.Logger) { l.Info("info message") },
			shouldLog:   false,
			description: "warn level should filter info messages",
		},
		{
			name:        "warn level logs warn messages",
			level:       logger.LevelWarn,
			logFunc:     func(l *logger.Logger) { l.Warn("warn message") },
			shouldLog:   true,
			description: "warn level should include warn messages",
		},
		{
			name:        "error level filters warn messages",
			level:       logger.LevelError,
			logFunc:     func(l *logger.Logger) { l.Warn("warn message") },
			shouldLog:   false,
			description: "error level should filter warn messages",
		},
		{
			name:        "error level logs error messages",
			level:       logger.LevelError,
			logFunc:     func(l *logger.Logger) { l.Error("error message") },
			shouldLog:   true,
			description: "error level should include error messages",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(tt.level, logger.JSON, buf)

			tt.logFunc(log)

			if tt.shouldLog {
				assert.NotEmpty(t, buf.String(), tt.description)
			} else {
				assert.Empty(t, buf.String(), tt.description)
			}
		})
	}
}

func TestContextFields(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(*logger.Logger, *bytes.Buffer)
		expected map[string]any
	}{
		{
			name: "With adds context fields",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				contextLogger := l.With("requestID", "123", "userID", "456")
				contextLogger.Info("request processed")
			},
			expected: map[string]any{
				"msg":       "request processed",
				"requestID": "123",
				"userID":    "456",
			},
		},
		{
			name: "multiple With calls chain context",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l1 := l.With("key1", "value1")
				l2 := l1.With("key2", "value2")
				l2.Info("chained context")
			},
			expected: map[string]any{
				"msg":  "chained context",
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "WithGroup groups fields",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				groupLogger := l.WithGroup("http")
				groupLogger.Info("request received", "method", "GET", "path", "/api")
			},
			expected: map[string]any{
				"msg": "request received",
				"http": map[string]any{
					"method": "GET",
					"path":   "/api",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelInfo, logger.JSON, buf)

			tt.logFunc(log, buf)

			var result map[string]any
			err := json.Unmarshal(buf.Bytes(), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expected["msg"], result["msg"])
			for k, v := range tt.expected {
				if k != "msg" {
					assert.Equal(t, v, result[k], "field %s should match", k)
				}
			}
		})
	}
}

func TestContextMethods(t *testing.T) {
	tests := []struct {
		name    string
		logFunc func(*logger.Logger, context.Context, *bytes.Buffer)
		level   string
	}{
		{
			name: "DebugContext logs with context",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.DebugContext(ctx, "debug with context", "key", "value")
			},
			level: "DEBUG",
		},
		{
			name: "InfoContext logs with context",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.InfoContext(ctx, "info with context", "key", "value")
			},
			level: "INFO",
		},
		{
			name: "WarnContext logs with context",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.WarnContext(ctx, "warn with context", "key", "value")
			},
			level: "WARN",
		},
		{
			name: "ErrorContext logs with context",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.ErrorContext(ctx, "error with context", "key", "value")
			},
			level: "ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelDebug, logger.JSON, buf)
			ctx := context.Background()

			tt.logFunc(log, ctx, buf)

			var result map[string]any
			err := json.Unmarshal(buf.Bytes(), &result)
			require.NoError(t, err)

			assert.Equal(t, tt.level, result["level"])
			assert.Contains(t, result["msg"], "with context")
			assert.Equal(t, "value", result["key"])
		})
	}
}

func TestInvalidLevel(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New("invalid", logger.JSON, buf)

	log.Debug("debug message")
	assert.Empty(t, buf.String(), "invalid level should default to info and filter debug")

	log.Info("info message")
	assert.NotEmpty(t, buf.String(), "invalid level should default to info")
}

func TestTimestampPresence(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New(logger.LevelInfo, logger.JSON, buf)

	log.Info("test message")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	timestamp, ok := result["time"].(string)
	require.True(t, ok, "timestamp should be a string")
	assert.NotEmpty(t, timestamp, "timestamp should not be empty")
	assert.True(t, strings.Contains(timestamp, "T"), "timestamp should be in ISO format")
}

func TestMultipleFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New(logger.LevelInfo, logger.JSON, buf)

	log.Info("operation completed",
		"duration", 123,
		"status", "success",
		"count", 42,
		"ratio", 0.95,
	)

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)

	assert.Equal(t, "operation completed", result["msg"])
	assert.Equal(t, float64(123), result["duration"])
	assert.Equal(t, "success", result["status"])
	assert.Equal(t, float64(42), result["count"])
	assert.Equal(t, 0.95, result["ratio"])
}

func TestTextFormat(t *testing.T) {
	tests := []struct {
		name             string
		logFunc          func(*logger.Logger, *bytes.Buffer)
		expectedContains []string
	}{
		{
			name: "debug log produces text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Debug("test message", "key", "value")
			},
			expectedContains: []string{"DEBUG", "test message", "key=value"},
		},
		{
			name: "info log produces text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Info("test message", "key", "value")
			},
			expectedContains: []string{"INFO", "test message", "key=value"},
		},
		{
			name: "warn log produces text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Warn("test message", "key", "value")
			},
			expectedContains: []string{"WARN", "test message", "key=value"},
		},
		{
			name: "error log produces text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l.Error("test message", "key", "value")
			},
			expectedContains: []string{"ERROR", "test message", "key=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelDebug, logger.TEXT, buf)

			tt.logFunc(log, buf)

			output := buf.String()
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestTextHandlerLevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		level     logger.Level
		logFunc   func(*logger.Logger)
		shouldLog bool
	}{
		{
			name:      "info level filters debug in text handler",
			level:     logger.LevelInfo,
			logFunc:   func(l *logger.Logger) { l.Debug("debug message") },
			shouldLog: false,
		},
		{
			name:      "info level logs info in text handler",
			level:     logger.LevelInfo,
			logFunc:   func(l *logger.Logger) { l.Info("info message") },
			shouldLog: true,
		},
		{
			name:      "warn level filters info in text handler",
			level:     logger.LevelWarn,
			logFunc:   func(l *logger.Logger) { l.Info("info message") },
			shouldLog: false,
		},
		{
			name:      "error level logs error in text handler",
			level:     logger.LevelError,
			logFunc:   func(l *logger.Logger) { l.Error("error message") },
			shouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(tt.level, logger.TEXT, buf)

			tt.logFunc(log)

			if tt.shouldLog {
				assert.NotEmpty(t, buf.String())
			} else {
				assert.Empty(t, buf.String())
			}
		})
	}
}

func TestTextHandlerContextFields(t *testing.T) {
	tests := []struct {
		name             string
		logFunc          func(*logger.Logger, *bytes.Buffer)
		expectedContains []string
	}{
		{
			name: "With adds context fields in text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				contextLogger := l.With("requestID", "123", "userID", "456")
				contextLogger.Info("request processed")
			},
			expectedContains: []string{"request processed", "requestID=123", "userID=456"},
		},
		{
			name: "multiple With calls chain context in text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				l1 := l.With("key1", "value1")
				l2 := l1.With("key2", "value2")
				l2.Info("chained context")
			},
			expectedContains: []string{"chained context", "key1=value1", "key2=value2"},
		},
		{
			name: "WithGroup groups fields in text format",
			logFunc: func(l *logger.Logger, buf *bytes.Buffer) {
				groupLogger := l.WithGroup("http")
				groupLogger.Info("request received", "method", "GET", "path", "/api")
			},
			expectedContains: []string{"request received", "http.method=GET", "http.path=/api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelInfo, logger.TEXT, buf)

			tt.logFunc(log, buf)

			output := buf.String()
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestTextHandlerContextMethods(t *testing.T) {
	tests := []struct {
		name             string
		logFunc          func(*logger.Logger, context.Context, *bytes.Buffer)
		expectedContains []string
	}{
		{
			name: "DebugContext logs with context in text format",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.DebugContext(ctx, "debug with context", "key", "value")
			},
			expectedContains: []string{"DEBUG", "debug with context", "key=value"},
		},
		{
			name: "InfoContext logs with context in text format",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.InfoContext(ctx, "info with context", "key", "value")
			},
			expectedContains: []string{"INFO", "info with context", "key=value"},
		},
		{
			name: "WarnContext logs with context in text format",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.WarnContext(ctx, "warn with context", "key", "value")
			},
			expectedContains: []string{"WARN", "warn with context", "key=value"},
		},
		{
			name: "ErrorContext logs with context in text format",
			logFunc: func(l *logger.Logger, ctx context.Context, buf *bytes.Buffer) {
				l.ErrorContext(ctx, "error with context", "key", "value")
			},
			expectedContains: []string{"ERROR", "error with context", "key=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			log := logger.New(logger.LevelDebug, logger.TEXT, buf)
			ctx := context.Background()

			tt.logFunc(log, ctx, buf)

			output := buf.String()
			for _, expected := range tt.expectedContains {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func TestTextHandlerMultipleFields(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New(logger.LevelInfo, logger.TEXT, buf)

	log.Info("operation completed",
		"duration", 123,
		"status", "success",
		"count", 42,
		"ratio", 0.95,
	)

	output := buf.String()
	assert.Contains(t, output, "operation completed")
	assert.Contains(t, output, "duration=123")
	assert.Contains(t, output, "status=success")
	assert.Contains(t, output, "count=42")
	assert.Contains(t, output, "ratio=0.95")
}

func TestInvalidHandlerTypeDefaultsToJSON(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New(logger.LevelInfo, "invalid", buf)

	log.Info("test message", "key", "value")

	var result map[string]any
	err := json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "invalid handler should default to JSON")

	assert.Equal(t, "INFO", result["level"])
	assert.Equal(t, "test message", result["msg"])
	assert.Equal(t, "value", result["key"])
}

func TestLevelFrom(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected logger.Level
	}{
		{
			name:     "valid debug level",
			input:    "debug",
			expected: logger.LevelDebug,
		},
		{
			name:     "valid info level",
			input:    "info",
			expected: logger.LevelInfo,
		},
		{
			name:     "valid warn level",
			input:    "warn",
			expected: logger.LevelWarn,
		},
		{
			name:     "valid error level",
			input:    "error",
			expected: logger.LevelError,
		},
		{
			name:     "invalid level defaults to info",
			input:    "invalid",
			expected: logger.LevelInfo,
		},
		{
			name:     "empty string defaults to info",
			input:    "",
			expected: logger.LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logger.LevelFrom(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatFunction(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected logger.LogHandler
	}{
		{
			name:     "valid JSON format",
			input:    "json",
			expected: logger.JSON,
		},
		{
			name:     "valid text format",
			input:    "text",
			expected: logger.TEXT,
		},
		{
			name:     "invalid format defaults to JSON",
			input:    "invalid",
			expected: logger.JSON,
		},
		{
			name:     "empty string defaults to JSON",
			input:    "",
			expected: logger.JSON,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := logger.FormatFrom(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewFromEnvs(t *testing.T) {
	t.Run("creates logger from environment variables", func(t *testing.T) {
		log := logger.NewFromEnvs()
		assert.NotNil(t, log)
	})
}

func TestStop(t *testing.T) {
	buf := &bytes.Buffer{}
	log := logger.New(logger.LevelInfo, logger.JSON, buf)

	err := log.Stop()
	assert.NoError(t, err)
}
