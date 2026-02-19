package sleep

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// newTestLogger returns a logger suitable for tests.
// Error level is used to keep test output clean.
func newTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// newBehaviorTestClient bypasses SSRF validation and directly instantiates a Client
// struct for HTTP behaviour tests.  httptest.Server always binds to 127.0.0.1, which
// the SSRF guard correctly rejects — this helper lets us test the HTTP layer in isolation.
func newBehaviorTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	return &Client{
		endpoint:   serverURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		logger:     newTestLogger(),
	}
}

// newBehaviorTestClientWithAuth is like newBehaviorTestClient but also sets an auth token.
func newBehaviorTestClientWithAuth(t *testing.T, serverURL, authToken string) *Client {
	t.Helper()
	return &Client{
		endpoint:   serverURL,
		authToken:  authToken,
		httpClient: &http.Client{Timeout: defaultTimeout},
		logger:     newTestLogger(),
	}
}

// --- NewClient construction tests ---

// TestNewClientSuccess verifies that a valid config creates a client without error.
// Uses a non-loopback hostname so SSRF validation passes without a real connection.
func TestNewClientSuccess(t *testing.T) {
	// Given: a valid config with a non-loopback http endpoint
	config := NewClientConfig("http://homeserver.local/sleep")

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: client is created successfully
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

// TestNewClientEmptyEndpoint verifies that an empty endpoint is rejected.
func TestNewClientEmptyEndpoint(t *testing.T) {
	// Given: a config with an empty endpoint
	config := ClientConfig{Endpoint: "", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrEmptyEndpoint
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrEmptyEndpoint))
	assert.Nil(t, client)
}

// TestNewClientInvalidSchemeFile verifies that a file:// URL is rejected.
func TestNewClientInvalidSchemeFile(t *testing.T) {
	// Given: a config with a file:// endpoint
	config := ClientConfig{Endpoint: "file:///etc/passwd", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrInvalidEndpoint
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidEndpoint))
	assert.Nil(t, client)
}

// TestNewClientInvalidSchemeFtp verifies that an ftp:// URL is rejected.
func TestNewClientInvalidSchemeFtp(t *testing.T) {
	// Given: a config with a ftp:// endpoint
	config := ClientConfig{Endpoint: "ftp://example.com/sleep", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrInvalidEndpoint
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidEndpoint))
	assert.Nil(t, client)
}

// TestNewClientLocalhostBlocked verifies that a localhost endpoint is rejected to prevent SSRF.
func TestNewClientLocalhostBlocked(t *testing.T) {
	// Given: a config targeting localhost
	config := ClientConfig{Endpoint: "http://localhost/sleep", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrSSRFAttempt
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSSRFAttempt))
	assert.Nil(t, client)
}

// TestNewClientLoopbackIPv4Blocked verifies that 127.0.0.1 is rejected to prevent SSRF.
func TestNewClientLoopbackIPv4Blocked(t *testing.T) {
	// Given: a config targeting the IPv4 loopback address
	config := ClientConfig{Endpoint: "http://127.0.0.1/sleep", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrSSRFAttempt
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSSRFAttempt))
	assert.Nil(t, client)
}

// TestNewClientLoopbackIPv6Blocked verifies that [::1] is rejected to prevent SSRF.
func TestNewClientLoopbackIPv6Blocked(t *testing.T) {
	// Given: a config targeting the IPv6 loopback address
	config := ClientConfig{Endpoint: "http://[::1]/sleep", Timeout: defaultTimeout}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrSSRFAttempt
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSSRFAttempt))
	assert.Nil(t, client)
}

// TestNewClientZeroTimeoutRejected verifies that a zero timeout is rejected.
func TestNewClientZeroTimeoutRejected(t *testing.T) {
	// Given: a config with a zero timeout
	config := ClientConfig{Endpoint: "http://homeserver.local/sleep", Timeout: 0}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrInvalidTimeout
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTimeout))
	assert.Nil(t, client)
}

// TestNewClientNegativeTimeoutRejected verifies that a negative timeout is rejected.
func TestNewClientNegativeTimeoutRejected(t *testing.T) {
	// Given: a config with a negative timeout
	config := ClientConfig{Endpoint: "http://homeserver.local/sleep", Timeout: -time.Second}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: should fail with ErrInvalidTimeout
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTimeout))
	assert.Nil(t, client)
}

// TestNewClientNilLoggerRejected verifies that a nil logger is rejected.
func TestNewClientNilLoggerRejected(t *testing.T) {
	// Given: a nil logger
	config := NewClientConfig("http://homeserver.local/sleep")

	// When: creating the client
	client, err := NewClient(config, nil)

	// Then: should fail with ErrNilLogger
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNilLogger))
	assert.Nil(t, client)
}

// TestNewClientConfigDefaultTimeout verifies that NewClientConfig sets the default timeout.
func TestNewClientConfigDefaultTimeout(t *testing.T) {
	// When: creating config via constructor
	config := NewClientConfig("http://homeserver.local/sleep")

	// Then: the default timeout is set
	assert.Equal(t, defaultTimeout, config.Timeout)
	assert.Equal(t, "http://homeserver.local/sleep", config.Endpoint)
}

// --- Sleep behaviour tests ---

// TestClientSleepSuccess verifies that a 200 response results in no error.
func TestClientSleepSuccess(t *testing.T) {
	// Given: a server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "must use GET method")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: no error
	assert.NoError(t, err)
}

// TestClientSleepSuccessOnAccepted verifies that a 202 Accepted response results in no error.
func TestClientSleepSuccessOnAccepted(t *testing.T) {
	// Given: a server that returns 202 Accepted
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: no error
	assert.NoError(t, err)
}

// TestClientSleepServerError verifies that a 500 response returns ErrSleepRequestFailed.
func TestClientSleepServerError(t *testing.T) {
	// Given: a server that returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: should fail with ErrSleepRequestFailed
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepRequestFailed))
}

// TestClientSleepClientError verifies that a 400 response returns ErrSleepRequestFailed.
func TestClientSleepClientError(t *testing.T) {
	// Given: a server that returns 400 Bad Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: should fail with ErrSleepRequestFailed
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrSleepRequestFailed))
}

// TestClientSleepNetworkError verifies that a network failure returns ErrNetworkError.
func TestClientSleepNetworkError(t *testing.T) {
	// Given: a client configured with an unreachable address
	config := ClientConfig{
		Endpoint: "http://192.0.2.1:9999", // TEST-NET-1, guaranteed unreachable
		Timeout:  50 * time.Millisecond,   // short timeout to fail quickly
	}
	client, err := NewClient(config, newTestLogger())
	assert.NoError(t, err)

	// When: sending a sleep command
	err = client.Sleep(context.Background())

	// Then: should fail with ErrNetworkError
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrNetworkError))
}

// TestClientSleepContextCancellation verifies that a cancelled context returns an error.
func TestClientSleepContextCancellation(t *testing.T) {
	// Given: a slow server and a pre-cancelled context
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// When: sending a sleep command with a cancelled context
	err := client.Sleep(ctx)

	// Then: should fail with some error
	assert.Error(t, err)
}

// TestClientSleepUsesGetMethod verifies that Sleep sends a GET request.
func TestClientSleepUsesGetMethod(t *testing.T) {
	// Given: a server that records the request method
	var receivedMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL)

	// When: sending a sleep command
	_ = client.Sleep(context.Background())

	// Then: the request must use GET
	assert.Equal(t, http.MethodGet, receivedMethod)
}

// TestClientSleepSendsAuthorizationHeaderWhenTokenSet verifies that Sleep sets
// the Authorization: Basic header when an auth token is configured.
func TestClientSleepSendsAuthorizationHeaderWhenTokenSet(t *testing.T) {
	// Given: a server that captures the Authorization header
	const token = "dXNlcjpwYXNzd29yZA==" //nolint:gosec // test-only dummy credential
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBehaviorTestClientWithAuth(t, server.URL, token)

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: Authorization header is set with the correct Basic token
	assert.NoError(t, err)
	assert.Equal(t, "Basic "+token, receivedAuth)
}

// TestClientSleepOmitsAuthorizationHeaderWhenTokenEmpty verifies that Sleep does
// not set an Authorization header when no auth token is configured.
func TestClientSleepOmitsAuthorizationHeaderWhenTokenEmpty(t *testing.T) {
	// Given: a server that captures the Authorization header
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newBehaviorTestClient(t, server.URL) // no auth token

	// When: sending a sleep command
	err := client.Sleep(context.Background())

	// Then: no Authorization header is sent
	assert.NoError(t, err)
	assert.Empty(t, receivedAuth)
}

// TestNewClientConfigAuthTokenIsStoredInClient verifies that AuthToken in ClientConfig
// is propagated to the constructed Client.
func TestNewClientConfigAuthTokenIsStoredInClient(t *testing.T) {
	// Given: a valid config with an auth token
	const token = "dXNlcjpwYXNzd29yZA==" //nolint:gosec // test-only dummy credential
	config := ClientConfig{
		Endpoint:  "http://homeserver.local/sleep",
		Timeout:   defaultTimeout,
		AuthToken: token,
	}

	// When: creating the client
	client, err := NewClient(config, newTestLogger())

	// Then: the client is created and stores the auth token
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, token, client.authToken)
}
