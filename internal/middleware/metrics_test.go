package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

const (
	metricsRequestsTotal    = "smaug_requests_total"
	metricsRequestsByStatus = "smaug_requests_by_status_total"
	metricsRequestsByMethod = "smaug_requests_by_method_total"
	metricsRequestDuration  = "smaug_request_duration_seconds"
)

func TestMetricsMiddlewareRecordsSuccessfulRequest(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify metrics were recorded
	families, err := reg.Gatherer().Gather()
	require.NoError(t, err)

	var requestsFound bool
	for _, mf := range families {
		if mf.GetName() == metricsRequestsTotal {
			requestsFound = true
			assert.NotNil(t, mf.Metric)
		}
	}
	assert.True(t, requestsFound, "smaug_requests_total metric not found")
}

func TestMetricsMiddlewareRecordsErrorResponse(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusNotFound, w.Code)
	assertMetricRecorded(t, reg, metricsRequestsByStatus)
}

func TestMetricsMiddlewareRecordsDuration(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify duration histogram was recorded
	families, err := reg.Gatherer().Gather()
	require.NoError(t, err)

	var durationFound bool
	for _, mf := range families {
		if mf.GetName() == metricsRequestDuration {
			durationFound = true
		}
	}
	assert.True(t, durationFound, "smaug_request_duration_seconds metric not found")
}

func TestMetricsMiddlewareRecordsHTTPMethod(t *testing.T) {
	// Given
	tests := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range tests {
		// Reset registry for each test
		reg, err := metrics.New()
		require.NoError(t, err)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := NewMetricsMiddleware(reg)
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest(method, "/test", nil)
		w := httptest.NewRecorder()

		// When
		wrappedHandler.ServeHTTP(w, req)

		// Then
		assert.Equal(t, http.StatusOK, w.Code)

		families, err := reg.Gatherer().Gather()
		require.NoError(t, err)

		methodFound := false
		for _, mf := range families {
			if mf.GetName() == metricsRequestsByMethod {
				methodFound = true
			}
		}
		assert.True(t, methodFound, "Method %s not recorded", method)
	}
}

func TestMetricsMiddlewareWithNilRegistry(t *testing.T) {
	// Given
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	middleware := NewMetricsMiddleware(nil)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// When (should not panic)
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMetricsMiddlewareDefaultStatusOK(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally don't call WriteHeader, should default to 200
		_, _ = w.Write([]byte("OK"))
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusOK, w.Code)

	families, err := reg.Gatherer().Gather()
	require.NoError(t, err)

	statusFound := false
	for _, mf := range families {
		if mf.GetName() == metricsRequestsByStatus {
			statusFound = true
		}
	}
	assert.True(t, statusFound, "Default status not recorded")
}

func TestMetricsMiddlewareConcurrentRequests(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	// When - make concurrent requests
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(w, req)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Then
	families, err := reg.Gatherer().Gather()
	require.NoError(t, err)

	// Verify metrics were recorded
	var found bool
	for _, mf := range families {
		if mf.GetName() == metricsRequestsTotal {
			found = true
			assert.NotNil(t, mf.Metric)
		}
	}
	assert.True(t, found, "smaug_requests_total not found")
}

func TestMetricsMiddlewareServerError(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})

	middleware := NewMetricsMiddleware(reg)
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodPost, "/error", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assertMetricRecorded(t, reg, metricsRequestsByStatus)
}

func TestMetricsResponseWriterFlush(t *testing.T) {
	// Given
	w := httptest.NewRecorder()
	wrapped := &MetricsResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// When - call Flush (should not panic even if underlying writer doesn't support it)
	wrapped.Flush()

	// Then - no panic means success
}

func TestMetricsResponseWriterUnwrap(t *testing.T) {
	// Given
	w := httptest.NewRecorder()
	wrapped := &MetricsResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}

	// When
	unwrapped := wrapped.Unwrap()

	// Then
	assert.Equal(t, w, unwrapped)
}

func TestMetricsResponseWriterMultipleWriteHeaders(t *testing.T) {
	// Given
	w := httptest.NewRecorder()
	wrapped := &MetricsResponseWriter{
		ResponseWriter: w,
	}

	// When - call WriteHeader twice
	wrapped.WriteHeader(http.StatusOK)
	wrapped.WriteHeader(http.StatusInternalServerError)

	// Then - only first call takes effect
	assert.Equal(t, http.StatusOK, wrapped.statusCode)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMetricsResponseWriterWriteImpliesWriteHeader(t *testing.T) {
	// Given
	w := httptest.NewRecorder()
	wrapped := &MetricsResponseWriter{
		ResponseWriter: w,
	}

	// When - call Write without WriteHeader
	n, err := wrapped.Write([]byte("test"))

	// Then
	assert.NoError(t, err)
	assert.Greater(t, n, 0)
	assert.Equal(t, http.StatusOK, wrapped.statusCode)
	assert.True(t, wrapped.written)
}

func TestNewMetricsMiddlewareIntegrationWithLoggingMiddleware(t *testing.T) {
	// Given
	reg, err := metrics.New()
	require.NoError(t, err)

	log := logger.New(logger.LevelInfo, logger.JSON, nil)

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Create chain with both middleware
	chain := Chain(
		NewMetricsMiddleware(reg),
		NewLoggingMiddleware(log),
	)
	wrappedHandler := chain(innerHandler)

	req := httptest.NewRequest(http.MethodGet, "/integrated", nil)
	w := httptest.NewRecorder()

	// When
	wrappedHandler.ServeHTTP(w, req)

	// Then
	assert.Equal(t, http.StatusOK, w.Code)
	assertMetricRecorded(t, reg, metricsRequestsTotal)
}

// assertMetricRecorded verifies that a specific metric was recorded in the registry.
func assertMetricRecorded(t *testing.T, reg *metrics.Registry, metricName string) {
	t.Helper()
	families, err := reg.Gatherer().Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() == metricName {
			return
		}
	}
	assert.Fail(t, "metric not recorded", "metric %s not found", metricName)
}
