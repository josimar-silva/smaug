package middleware

import (
	"net/http"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

// MetricsResponseWriter wraps http.ResponseWriter to capture the status code for metrics recording.
type MetricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

// WriteHeader captures the status code before delegating to the underlying ResponseWriter.
func (w *MetricsResponseWriter) WriteHeader(statusCode int) {
	if w.written {
		return
	}
	w.written = true
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write ensures WriteHeader is called before writing, then delegates to the underlying ResponseWriter.
func (w *MetricsResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher if the underlying writer supports it.
func (w *MetricsResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for middleware-aware libraries.
func (w *MetricsResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// NewMetricsMiddleware creates a middleware that records HTTP request metrics.
// It captures request method, response status code, and request duration.
// The metrics are recorded via the provided metrics registry.
func NewMetricsMiddleware(m *metrics.Registry) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap response writer to capture status code
			wrapped := &MetricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
			}

			// Record start time
			startTime := time.Now()

			// Handle request
			next.ServeHTTP(wrapped, r)

			// Record metrics (fail-safe, won't panic on nil)
			if m != nil {
				duration := time.Since(startTime).Seconds()
				m.Request.RecordRequest(r.Method, wrapped.statusCode, duration)
			}
		})
	}
}
