package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

// RequestIDKey is the context key for request ID.
const RequestIDKey contextKey = "request_id"

// LoggingResponseWriter wraps http.ResponseWriter to capture status code and bytes written.
type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	bytesWritten  int
	headerWritten bool
}

// WriteHeader captures the status code and delegates to the underlying ResponseWriter.
// It guards against multiple calls, only honoring the first one.
func (w *LoggingResponseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write captures bytes written and delegates to the underlying ResponseWriter.
// It ensures a default status code is set if WriteHeader was not called explicitly.
func (w *LoggingResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

// Flush implements http.Flusher by delegating to the underlying writer if supported.
// This is necessary for server-sent events (SSE) and streaming responses.
func (w *LoggingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter, enabling middleware-aware
// libraries to recover optional interfaces.
func (w *LoggingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Middleware is a function that wraps an http.Handler to add cross-cutting concerns.
type Middleware func(http.Handler) http.Handler

// NewLoggingMiddleware creates a middleware that logs HTTP requests and responses
// with structured logging. It captures request method, path, query parameters,
// client IP, response status code, response size, and request duration.
// Each request is assigned a unique request_id for correlation, which is added
// to both the request context (for downstream handlers) and response headers
// (for client reference).
func NewLoggingMiddleware(log *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := uuid.New().String()

			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
			r = r.WithContext(ctx)
			w.Header().Set("X-Request-ID", requestID)

			clientIP := extractClientIP(r)

			wrapped := &LoggingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
			}

			startTime := time.Now()

			log.InfoContext(
				r.Context(),
				"request",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"query", r.URL.RawQuery,
				"client_ip", clientIP,
			)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(startTime)

			log.InfoContext(
				r.Context(),
				"response",
				"request_id", requestID,
				"status", wrapped.statusCode,
				"response_bytes", wrapped.bytesWritten,
				"duration", duration,
			)
		})
	}
}

// extractClientIP extracts the client IP address from request headers or RemoteAddr.
// It first checks X-Forwarded-For (taking the leftmost entry), then X-Real-IP,
// and finally falls back to RemoteAddr. Only trust X-Forwarded-For and X-Real-IP
// if the service is behind a controlled proxy/load balancer.
func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.Index(xff, ","); idx > 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	return extractIPFromRemoteAddr(r.RemoteAddr)
}

// extractIPFromRemoteAddr extracts the IP address from RemoteAddr, removing the port if present.
func extractIPFromRemoteAddr(remoteAddr string) string {
	if strings.Contains(remoteAddr, ":") {
		ip, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			return remoteAddr
		}
		return ip
	}
	return remoteAddr
}

// Chain combines multiple middleware into a single middleware that applies them in order.
// The first middleware in the chain is the outermost, so it will execute first for requests
// and last for responses.
func Chain(middlewares ...Middleware) Middleware {
	return func(handler http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}
