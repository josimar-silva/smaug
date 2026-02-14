package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// LevelFrom creates a Level from a string value.
// Returns LevelInfo if the value is invalid.
func LevelFrom(s string) Level {
	switch Level(s) {
	case LevelDebug:
		return LevelDebug
	case LevelWarn:
		return LevelWarn
	case LevelError:
		return LevelError
	default:
		return LevelInfo
	}
}

type LogHandler string

const (
	JSON LogHandler = "json"
	TEXT LogHandler = "text"
)

// FormatFrom creates a LogHandler from a string value.
// Returns JSON if the value is invalid.
func FormatFrom(s string) LogHandler {
	switch LogHandler(s) {
	case TEXT:
		return TEXT
	default:
		return JSON
	}
}

type Logger struct {
	logger *slog.Logger
}

// New creates a new Logger with the specified level, handler type, and output.
// If output is nil, stdout is used.
func New(level Level, logHandler LogHandler, output io.Writer) *Logger {
	if output == nil {
		output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	handler := createHandler(logHandler, output, opts)
	return &Logger{
		logger: slog.New(handler),
	}
}

// NewFromEnvs creates a new Logger with settings from environment variables.
// Uses sensible defaults if environment variables are not set:
//   - SMAUG_LOG_LEVEL: defaults to "info" (debug, info, warn, error)
//   - SMAUG_LOG_FORMAT: defaults to "json" (json or text)
//   - Output: always stdout
func NewFromEnvs() *Logger {
	level := getLogLevelFromEnv()
	format := getLogFormatFromEnv()
	return New(level, format, nil)
}

func getLogLevelFromEnv() Level {
	level := os.Getenv("SMAUG_LOG_LEVEL")
	return LevelFrom(level)
}

func getLogFormatFromEnv() LogHandler {
	format := os.Getenv("SMAUG_LOG_FORMAT")
	return FormatFrom(format)
}

func createHandler(handlerType LogHandler, output io.Writer, opts *slog.HandlerOptions) slog.Handler {
	switch handlerType {
	case TEXT:
		return slog.NewTextHandler(output, opts)
	default:
		return slog.NewJSONHandler(output, opts)
	}
}

func parseLevel(level Level) slog.Level {
	switch level {
	case LevelDebug:
		return slog.LevelDebug
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *Logger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l *Logger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *Logger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

func (l *Logger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		logger: l.logger.With(args...),
	}
}

func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		logger: l.logger.WithGroup(name),
	}
}

// Stop performs any necessary cleanup for the logger.
// This is a no-op for slog-based loggers, but provides a consistent
// interface for resource management and future enhancements.
func (l *Logger) Stop() error {
	return nil
}
