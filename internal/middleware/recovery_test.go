package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

func TestRecoveryMiddleware(t *testing.T) {
	// Given: A logger to capture output
	var buf bytes.Buffer
	log := logger.New(logger.LevelInfo, logger.JSON, &buf)

	// And a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	})

	// When: Wrapping with recovery middleware
	recovery := NewRecoveryMiddleware(log)
	handler := recovery(panicHandler)

	// And executing the request
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Then: Status code is 500
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// And response body is "Internal Server Error"
	assert.Equal(t, "Internal Server Error\n", rr.Body.String())

	// And logs contain panic message and stack trace
	logOutput := buf.String()
	assert.Contains(t, logOutput, "panic recovered")
	assert.Contains(t, logOutput, "something went wrong")
	assert.Contains(t, logOutput, "stack_trace")
}

func TestRecoveryMiddlewareNoPanic(t *testing.T) {
	// Given: A standard logger
	var buf bytes.Buffer
	log := logger.New(logger.LevelInfo, logger.JSON, &buf)

	// And a handler that works normally
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// When: Wrapping with recovery middleware
	recovery := NewRecoveryMiddleware(log)
	handler := recovery(okHandler)

	// And executing the request
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Then: Status code is 200
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())

	// And no panic logs
	logOutput := buf.String()
	assert.NotContains(t, logOutput, "panic recovered")
}

func TestRecoveryMiddlewarePanicWithNonString(t *testing.T) {
	// Given: A logger to capture output
	var buf bytes.Buffer
	log := logger.New(logger.LevelInfo, logger.JSON, &buf)

	// And a handler that panics with non-string value
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(123)
	})

	// When: Wrapping with recovery middleware
	recovery := NewRecoveryMiddleware(log)
	handler := recovery(panicHandler)

	// And executing the request
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	// Then: Status code is 500
	assert.Equal(t, http.StatusInternalServerError, rr.Code)

	// And logs contain panic value
	logOutput := buf.String()
	assert.Contains(t, logOutput, "panic recovered")
	assert.Contains(t, logOutput, "123")
}
