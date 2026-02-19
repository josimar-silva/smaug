package sleep

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// Log field names as constants to avoid duplication.
const (
	logFieldEndpoint = "endpoint"
	logFieldError    = "error"
	logFieldStatus   = "status_code"
)

// Log messages as constants to avoid duplication.
const (
	logMsgSendingSleep       = "sending sleep command"
	logMsgSleepCommandSent   = "sleep command sent successfully"
	logMsgSleepRequestFailed = "sleep request failed"
	logMsgNetworkError       = "sleep request failed with network error"
)

// Error message templates.
const (
	errMsgNetworkError  = "%w: %s: %w"
	errMsgRequestFailed = "%w: %d"
	errMsgCreateRequest = "%w: failed to create request for %s: %w"
)

// Client is a REST client for sending sleep commands to a remote endpoint.
type Client struct {
	endpoint   string
	httpClient *http.Client
	logger     *logger.Logger
}

// NewClient creates a new sleep REST client with the given configuration.
//
// Parameters:
//   - config: Client configuration including the target endpoint and request timeout.
//     The endpoint must use the http or https scheme and must not resolve to a loopback
//     or localhost address (SSRF protection).
//   - logger: Structured logger for client operations.
//
// Possible errors:
//   - ErrEmptyEndpoint if config.Endpoint is empty
//   - ErrInvalidEndpoint if the endpoint scheme is not http or https
//   - ErrSSRFAttempt if the endpoint host is a loopback or localhost address
//   - ErrInvalidTimeout if config.Timeout is zero or negative
//   - ErrNilLogger if logger is nil
func NewClient(config ClientConfig, log *logger.Logger) (*Client, error) {
	if err := validateConfig(config, log); err != nil {
		return nil, err
	}

	return &Client{
		endpoint: config.Endpoint,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger: log,
	}, nil
}

func validateConfig(config ClientConfig, log *logger.Logger) error {
	if config.Endpoint == "" {
		return ErrEmptyEndpoint
	}

	if err := validateEndpoint(config.Endpoint); err != nil {
		return err
	}

	if config.Timeout <= 0 {
		return ErrInvalidTimeout
	}

	if log == nil {
		return ErrNilLogger
	}

	return nil
}

// validateEndpoint checks that the endpoint uses an allowed scheme and does not
// target a loopback or localhost address (SSRF protection).
func validateEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEndpoint, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: got %q", ErrInvalidEndpoint, parsed.Scheme)
	}

	host := parsed.Hostname()
	if isLoopbackOrLocalhost(host) {
		return fmt.Errorf("%w: %q", ErrSSRFAttempt, host)
	}

	return nil
}

// isLoopbackOrLocalhost returns true when host is "localhost", a loopback IP (127.x.x.x / ::1),
// or the unspecified address (0.0.0.0 / ::).
func isLoopbackOrLocalhost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() || ip.IsUnspecified()
}

// Sleep sends a GET request to the configured sleep endpoint.
// It logs the trigger and returns an error if the request fails or the
// endpoint returns a non-2xx status code.
func (c *Client) Sleep(ctx context.Context) error {
	c.logger.InfoContext(ctx, logMsgSendingSleep, logFieldEndpoint, c.endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, http.NoBody)
	if err != nil {
		return fmt.Errorf(errMsgCreateRequest, ErrNetworkError, c.endpoint, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WarnContext(ctx, logMsgNetworkError,
			logFieldEndpoint, c.endpoint,
			logFieldError, err,
		)
		return fmt.Errorf(errMsgNetworkError, ErrNetworkError, c.endpoint, err)
	}
	defer resp.Body.Close() //nolint:errcheck // ignoring body close error on read-only response

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.logger.WarnContext(ctx, logMsgSleepRequestFailed,
			logFieldEndpoint, c.endpoint,
			logFieldStatus, resp.StatusCode,
		)
		return fmt.Errorf(errMsgRequestFailed, ErrSleepRequestFailed, resp.StatusCode)
	}

	c.logger.InfoContext(ctx, logMsgSleepCommandSent,
		logFieldEndpoint, c.endpoint,
		logFieldStatus, resp.StatusCode,
	)

	return nil
}
