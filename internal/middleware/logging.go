package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// loggingResponseWriter wraps http.ResponseWriter to capture status code and bytes written.
type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader captures the status code and delegates to the underlying ResponseWriter.
func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write captures bytes written and delegates to the underlying ResponseWriter.
func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

// Middleware is a function that wraps an http.Handler to add cross-cutting concerns.
type Middleware func(http.Handler) http.Handler

// NewLoggingMiddleware creates a middleware that logs HTTP requests and responses
// with structured logging. It captures request method, path, query parameters,
// client IP, response status code, response size, and request duration.
// Each request is assigned a unique request_id for correlation.
func NewLoggingMiddleware(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate unique request ID
			requestID := uuid.New().String()

			// Extract client IP from RemoteAddr
			clientIP := extractClientIP(r.RemoteAddr)

			// Create logging response writer wrapper
			wrapped := &loggingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
			}

			// Start timing the request
			startTime := time.Now()

			// Log the incoming request
			logger.InfoContext(
				r.Context(),
				"request",
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.String("client_ip", clientIP),
			)

			// Call the next handler
			next.ServeHTTP(wrapped, r)

			// Calculate request duration
			duration := time.Since(startTime)

			// Log the response
			logger.InfoContext(
				r.Context(),
				"response",
				slog.String("request_id", requestID),
				slog.Int("status", wrapped.statusCode),
				slog.Int("response_bytes", wrapped.bytesWritten),
				slog.Duration("duration", duration),
			)
		})
	}
}

// extractClientIP extracts the client IP address from RemoteAddr, removing the port if present.
func extractClientIP(remoteAddr string) string {
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
		// Apply middleware in reverse order so the first in the slice is outermost
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}
