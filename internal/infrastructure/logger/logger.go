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

type LogHandler string

const (
	JSON LogHandler = "json"
	TEXT LogHandler = "text"
)

type Logger struct {
	logger *slog.Logger
}

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
