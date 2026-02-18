package gwaihir

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// newRetryTestLogger returns a logger suitable for retry tests.
// Error level is used to keep test output clean while still capturing
// any unexpected logger.Error calls that indicate bugs.
func newRetryTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// newClientWithRetry is a test helper that builds a Client pointing at the
// given server URL, with the supplied RetryConfig injected.
func newClientWithRetry(t *testing.T, serverURL string, retryConfig RetryConfig) *Client {
	t.Helper()
	config := ClientConfig{
		BaseURL:     serverURL,
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: retryConfig,
	}
	client, err := NewClient(config, newRetryTestLogger())
	assert.NoError(t, err)
	return client
}

// countingServer creates an httptest.Server whose handler invokes handleFn and
// increments *callCount on every request.
func countingServer(t *testing.T, callCount *atomic.Int32, handleFn http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		handleFn(w, r)
	}))
}

// writeJSONResponse writes a JSON-encoded WoLResponse with the given status and message.
func writeJSONResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(WoLResponse{Message: message})
}

// --- Retry configuration validation tests ---

// TestNewClientWithDefaultRetryConfig verifies that a client built with NewRetryConfig()
// carries the expected default values.
func TestNewClientWithDefaultRetryConfig(t *testing.T) {
	// Given: a ClientConfig with an explicit default RetryConfig
	config := ClientConfig{
		BaseURL:     "http://gwaihir.example.com",
		APIKey:      "test-api-key",
		Timeout:     5 * time.Second,
		RetryConfig: NewRetryConfig(),
	}

	// When: creating the client
	client, err := NewClient(config, newRetryTestLogger())

	// Then: the client should carry the default retry config values
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, defaultMaxAttempts, client.retryConfig.MaxAttempts)
	assert.Equal(t, defaultBaseDelay, client.retryConfig.BaseDelay)
}

// TestNewClientCustomRetryConfig verifies that an explicit RetryConfig is used as-is.
func TestNewClientCustomRetryConfig(t *testing.T) {
	// Given: a ClientConfig with a custom RetryConfig
	config := ClientConfig{
		BaseURL: "http://gwaihir.example.com",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
		RetryConfig: RetryConfig{
			MaxAttempts: 5,
			BaseDelay:   500 * time.Millisecond,
		},
	}

	// When: creating the client
	client, err := NewClient(config, newRetryTestLogger())

	// Then: the custom retry config must be preserved exactly
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, 5, client.retryConfig.MaxAttempts)
	assert.Equal(t, 500*time.Millisecond, client.retryConfig.BaseDelay)
}

// TestNewClientInvalidMaxAttempts verifies that MaxAttempts = 0 is rejected.
func TestNewClientInvalidMaxAttempts(t *testing.T) {
	// Given: a RetryConfig with zero MaxAttempts
	config := ClientConfig{
		BaseURL: "http://gwaihir.example.com",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
		RetryConfig: RetryConfig{
			MaxAttempts: 0,
			BaseDelay:   time.Second,
		},
	}

	// When: creating the client
	client, err := NewClient(config, newRetryTestLogger())

	// Then: should fail with ErrInvalidRetryConfig
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
	assert.Nil(t, client)
}

// TestNewClientInvalidBaseDelay verifies that a zero BaseDelay is rejected.
func TestNewClientInvalidBaseDelay(t *testing.T) {
	// Given: a RetryConfig with zero BaseDelay
	config := ClientConfig{
		BaseURL: "http://gwaihir.example.com",
		APIKey:  "test-api-key",
		Timeout: 5 * time.Second,
		RetryConfig: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   0,
		},
	}

	// When: creating the client
	client, err := NewClient(config, newRetryTestLogger())

	// Then: should fail with ErrInvalidRetryConfig
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
	assert.Nil(t, client)
}

// --- Retry behaviour tests ---

// TestClientSendWoLRetriesOnServerError verifies that a 500 response triggers a retry,
// and a subsequent success is returned to the caller without an error.
func TestClientSendWoLRetriesOnServerError(t *testing.T) {
	// Given: a server that fails once with 500 then succeeds on the second attempt
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		if callCount.Load() == 1 {
			writeJSONResponse(w, http.StatusInternalServerError, "transient error")
			return
		}
		writeJSONResponse(w, http.StatusAccepted, "WoL packet sent")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond, // fast for tests
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: the call should succeed after the retry
	assert.NoError(t, err)
	assert.Equal(t, int32(2), callCount.Load(), "expected exactly 2 HTTP calls")
}

// TestClientSendWoLRetriesOnBadGateway verifies that a 502 Bad Gateway is retried.
func TestClientSendWoLRetriesOnBadGateway(t *testing.T) {
	// Given: a server that always returns 502
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusBadGateway, "bad gateway")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: all 3 attempts must be made and the final error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Equal(t, int32(3), callCount.Load(), "expected all 3 attempts to be made")
}

// TestClientSendWoLRetriesOnServiceUnavailable verifies that a 503 is retried.
func TestClientSendWoLRetriesOnServiceUnavailable(t *testing.T) {
	// Given: a server that always returns 503
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusServiceUnavailable, "service unavailable")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: all attempts exhausted, error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Equal(t, int32(3), callCount.Load())
}

// TestClientSendWoLDoesNotRetryOn401 verifies that 401 Unauthorized is never retried.
func TestClientSendWoLDoesNotRetryOn401(t *testing.T) {
	// Given: a server that always returns 401
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusUnauthorized, "invalid API key")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: exactly one attempt, no retry, authentication error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthenticationFailed))
	assert.Equal(t, int32(1), callCount.Load(), "must not retry on 401")
}

// TestClientSendWoLDoesNotRetryOn404 verifies that 404 Not Found is never retried.
func TestClientSendWoLDoesNotRetryOn404(t *testing.T) {
	// Given: a server that always returns 404
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusNotFound, "machine not found")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "unknown-machine")

	// Then: exactly one attempt, no retry, machine not found error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMachineNotFound))
	assert.Equal(t, int32(1), callCount.Load(), "must not retry on 404")
}

// TestClientSendWoLDoesNotRetryOn400 verifies that 400 Bad Request is never retried.
func TestClientSendWoLDoesNotRetryOn400(t *testing.T) {
	// Given: a server that always returns 400
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusBadRequest, "invalid request")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: exactly one attempt, no retry, WoL request failed error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Equal(t, int32(1), callCount.Load(), "must not retry on 400")
}

// TestClientSendWoLRetriesOnNetworkError verifies that network-level failures are retried.
func TestClientSendWoLRetriesOnNetworkError(t *testing.T) {
	// Given: an unreachable server with a very short timeout to trigger network errors quickly
	config := ClientConfig{
		BaseURL: "http://192.0.2.1:9999", // TEST-NET-1, guaranteed unreachable
		APIKey:  "test-api-key",
		Timeout: 50 * time.Millisecond,
		RetryConfig: RetryConfig{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Millisecond,
		},
	}
	client, err := NewClient(config, newRetryTestLogger())
	assert.NoError(t, err)

	// When: sending a WoL command
	start := time.Now()
	err = client.SendWoL(context.Background(), "saruman")
	elapsed := time.Since(start)

	// Then: should fail with network error after all attempts
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNetworkError))
	// Sanity check: at least 2 delays occurred (attempt 1 failed → wait → attempt 2 failed → wait → attempt 3 failed)
	// With BaseDelay=1ms, total wait is at least 1ms+2ms=3ms, well within 2s
	assert.Less(t, elapsed, 2*time.Second, "retries should complete quickly with 1ms base delay")
}

// TestClientSendWoLExponentialBackoffTimings verifies that the delay doubles between attempts.
// The test uses a channel to record call timestamps so it can assert the inter-attempt
// delay follows the exponential pattern without relying on wall-clock sleeps in the test itself.
func TestClientSendWoLExponentialBackoffTimings(t *testing.T) {
	// Given: a server that always returns 500, and records the timestamp of each call
	timestamps := make(chan time.Time, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		timestamps <- time.Now()
		writeJSONResponse(w, http.StatusInternalServerError, "error")
	}))
	defer server.Close()

	baseDelay := 50 * time.Millisecond
	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   baseDelay,
	})

	// When: sending a WoL command (will exhaust retries)
	err := client.SendWoL(context.Background(), "saruman")
	close(timestamps)

	// Then: error is returned and backoff timings are roughly correct
	assert.Error(t, err)

	var times []time.Time
	for ts := range timestamps {
		times = append(times, ts)
	}
	assert.Len(t, times, 3, "expected 3 attempts")

	// Gap between attempt 1 and 2 should be >= baseDelay (50ms)
	gap1 := times[1].Sub(times[0])
	assert.GreaterOrEqual(t, gap1, baseDelay,
		"first retry delay should be at least baseDelay (%v), got %v", baseDelay, gap1)

	// Gap between attempt 2 and 3 should be >= 2*baseDelay (100ms)
	gap2 := times[2].Sub(times[1])
	assert.GreaterOrEqual(t, gap2, 2*baseDelay,
		"second retry delay should be at least 2*baseDelay (%v), got %v", 2*baseDelay, gap2)
}

// TestClientSendWoLContextCancelledDuringRetry verifies that context cancellation
// mid-retry stops the retry loop and returns an appropriate error immediately.
func TestClientSendWoLContextCancelledDuringRetry(t *testing.T) {
	// Given: a server that always returns 500, and a context that is cancelled quickly
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusInternalServerError, "error")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   500 * time.Millisecond, // long delay so we can cancel in it
	})

	ctx, cancel := context.WithCancel(context.Background())

	// When: sending a WoL command and cancelling the context shortly after the first attempt
	done := make(chan error, 1)
	go func() {
		done <- client.SendWoL(ctx, "saruman")
	}()

	// Cancel after the first attempt has had time to complete but before the backoff elapses
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done

	// Then: an error is returned (context-cancelled or network-error), not nil
	assert.Error(t, err)
	// The retry loop must not have made all 5 attempts — it should have stopped early
	assert.Less(t, callCount.Load(), int32(5), "retry loop must stop when context is cancelled")
}

// TestClientSendWoLSucceedsAfterMultipleTransientErrors verifies that a success on
// the final allowed attempt is treated as overall success.
func TestClientSendWoLSucceedsAfterMultipleTransientErrors(t *testing.T) {
	// Given: a server that fails with 500 twice then succeeds on the third attempt
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		if callCount.Load() < 3 {
			writeJSONResponse(w, http.StatusServiceUnavailable, "temporarily unavailable")
			return
		}
		writeJSONResponse(w, http.StatusAccepted, "WoL packet sent")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: should succeed on the third attempt
	assert.NoError(t, err)
	assert.Equal(t, int32(3), callCount.Load())
}

// TestClientSendWoLAllAttemptsExhausted verifies that when all attempts fail with a
// transient error, ErrWoLRequestFailed is returned.
func TestClientSendWoLAllAttemptsExhausted(t *testing.T) {
	// Given: a server that always returns 500
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusInternalServerError, "persistent error")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: all 3 attempts made, error returned
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Equal(t, int32(3), callCount.Load(), "all MaxAttempts must be exhausted")
}

// TestClientSendWoLSingleAttemptConfig verifies that MaxAttempts=1 behaves like the
// original client with no retry: one call, immediate failure on error.
func TestClientSendWoLSingleAttemptConfig(t *testing.T) {
	// Given: a server that returns 500 and a client configured for 1 attempt only
	var callCount atomic.Int32
	server := countingServer(t, &callCount, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusInternalServerError, "error")
	})
	defer server.Close()

	client := newClientWithRetry(t, server.URL, RetryConfig{
		MaxAttempts: 1,
		BaseDelay:   1 * time.Millisecond,
	})

	// When: sending a WoL command
	err := client.SendWoL(context.Background(), "saruman")

	// Then: exactly 1 attempt, no retry
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrWoLRequestFailed))
	assert.Equal(t, int32(1), callCount.Load())
}

// --- Unit tests for RetryConfig methods ---

// TestRetryConfigBackoffDoubles verifies that each successive retry doubles the delay.
func TestRetryConfigBackoffDoubles(t *testing.T) {
	base := 100 * time.Millisecond
	rc := RetryConfig{MaxAttempts: 3, BaseDelay: base}

	// retry=1 → 1×base, retry=2 → 2×base, retry=3 → 4×base
	// (jitter is at most 10%, so we check lower bounds only)
	delay1 := rc.Backoff(1)
	delay2 := rc.Backoff(2)
	delay3 := rc.Backoff(3)

	assert.GreaterOrEqual(t, delay1, base, "retry 1 must be at least 1×base")
	assert.GreaterOrEqual(t, delay2, 2*base, "retry 2 must be at least 2×base")
	assert.GreaterOrEqual(t, delay3, 4*base, "retry 3 must be at least 4×base")
}

// TestRetryConfigBackoffCapAt60Seconds verifies that the delay is capped at 60 s regardless
// of how many retries have occurred.
func TestRetryConfigBackoffCapAt60Seconds(t *testing.T) {
	// Given: a base delay that would produce a huge value after many doublings
	base := 1 * time.Second
	rc := RetryConfig{MaxAttempts: 10, BaseDelay: base}

	// retry=7 produces 6 doublings (64s), which should be capped at 60s.
	delay := rc.Backoff(7)

	// Then: delay must be capped (60s + up to 10% jitter = at most 66s)
	assert.LessOrEqual(t, delay, maxBackoffDelay+maxBackoffDelay/10,
		"capped delay should not exceed 60s + 10%% jitter")
	assert.GreaterOrEqual(t, delay, maxBackoffDelay,
		"capped delay must be at least the cap value")
}

// TestRetryConfigBackoffCapsOversizedBaseDelay verifies that when BaseDelay itself
// exceeds maxBackoffDelay, Backoff(1) is still capped at maxBackoffDelay and never
// returns a value larger than subsequent retries (which would be paradoxical).
func TestRetryConfigBackoffCapsOversizedBaseDelay(t *testing.T) {
	// Given: a BaseDelay that already exceeds the cap
	rc := RetryConfig{MaxAttempts: 3, BaseDelay: 2 * maxBackoffDelay}

	// When: computing the first and second retry delay
	delay1 := rc.Backoff(1)
	delay2 := rc.Backoff(2)

	// Then: both delays must be capped (max 60s + 10% jitter = 66s)
	maxAllowed := maxBackoffDelay + maxBackoffDelay/10
	assert.LessOrEqual(t, delay1, maxAllowed, "Backoff(1) must be capped when BaseDelay > maxBackoffDelay")
	assert.LessOrEqual(t, delay2, maxAllowed, "Backoff(2) must be capped when BaseDelay > maxBackoffDelay")

	// And: the first retry must not be longer than the second (no paradox)
	assert.LessOrEqual(t, delay1, delay2+maxBackoffDelay/10,
		"Backoff(1) must not exceed Backoff(2) by more than jitter tolerance")
}

// TestRetryConfigBackoffJitterIsNonNegative verifies that the jitter never makes the
// delay shorter than the expected minimum.
func TestRetryConfigBackoffJitterIsNonNegative(t *testing.T) {
	base := 50 * time.Millisecond
	rc := RetryConfig{MaxAttempts: 3, BaseDelay: base}
	// Run several times to get different random values.
	for i := 0; i < 20; i++ {
		delay := rc.Backoff(1)
		assert.GreaterOrEqual(t, delay, base, "jitter must never reduce the delay below base")
	}
}

// TestSleepWithContextCancelled verifies that sleepWithContext returns ctx.Err() immediately
// when the context is already cancelled.
func TestSleepWithContextCancelled(t *testing.T) {
	// Given: a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When: sleeping with the cancelled context
	start := time.Now()
	err := sleepWithContext(ctx, 10*time.Second)
	elapsed := time.Since(start)

	// Then: returns immediately with a context error
	assert.Error(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "must return immediately on cancelled context")
}

// TestSleepWithContextCompletes verifies that sleepWithContext returns nil after the
// duration elapses when the context is not cancelled.
func TestSleepWithContextCompletes(t *testing.T) {
	// Given: a valid context and a very short sleep
	ctx := context.Background()

	// When: sleeping for 1ms
	err := sleepWithContext(ctx, 1*time.Millisecond)

	// Then: no error, sleep completed normally
	assert.NoError(t, err)
}

// TestRetryConfigValidateAcceptsValidConfig verifies that a well-formed RetryConfig
// passes validation.
func TestRetryConfigValidateAcceptsValidConfig(t *testing.T) {
	// Given: a valid RetryConfig
	rc := RetryConfig{MaxAttempts: 3, BaseDelay: time.Second}

	// When: validating
	err := rc.Validate()

	// Then: no error
	assert.NoError(t, err)
}

// TestRetryConfigValidateRejectsNegativeMaxAttempts verifies that a negative MaxAttempts
// value is rejected with ErrInvalidRetryConfig.
func TestRetryConfigValidateRejectsNegativeMaxAttempts(t *testing.T) {
	// Given: a RetryConfig with negative MaxAttempts
	rc := RetryConfig{MaxAttempts: -1, BaseDelay: time.Second}

	// When: validating
	err := rc.Validate()

	// Then: error wrapping ErrInvalidRetryConfig
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
}

// TestRetryConfigValidateRejectsNegativeBaseDelay verifies that a negative BaseDelay
// is rejected with ErrInvalidRetryConfig.
func TestRetryConfigValidateRejectsNegativeBaseDelay(t *testing.T) {
	// Given: a RetryConfig with negative BaseDelay
	rc := RetryConfig{MaxAttempts: 3, BaseDelay: -time.Second}

	// When: validating
	err := rc.Validate()

	// Then: error wrapping ErrInvalidRetryConfig
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidRetryConfig))
}

// TestIsNonRetryableStatusCovers4xx verifies that all relevant 4xx codes are non-retryable.
func TestIsNonRetryableStatusCovers4xx(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 409, 422, 429} {
		assert.True(t, isNonRetryableStatus(code), "status %d should be non-retryable", code)
	}
}

// TestIsNonRetryableStatusAllows5xx verifies that 5xx codes are NOT flagged as non-retryable.
func TestIsNonRetryableStatusAllows5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		assert.False(t, isNonRetryableStatus(code), "status %d should be retryable", code)
	}
}

// TestIsNonRetryableStatusAllows2xx verifies that 2xx codes are NOT flagged as non-retryable.
func TestIsNonRetryableStatusAllows2xx(t *testing.T) {
	for _, code := range []int{200, 201, 202, 204} {
		assert.False(t, isNonRetryableStatus(code), "status %d should not be flagged as non-retryable", code)
	}
}

// TestNewRetryConfigReturnsDefaults verifies that the constructor pre-populates
// the package defaults so callers do not have to specify values explicitly.
func TestNewRetryConfigReturnsDefaults(t *testing.T) {
	// When: creating a RetryConfig via the constructor
	rc := NewRetryConfig()

	// Then: the package defaults are returned and the config is valid
	assert.Equal(t, defaultMaxAttempts, rc.MaxAttempts)
	assert.Equal(t, defaultBaseDelay, rc.BaseDelay)
	assert.NoError(t, rc.Validate())
}
