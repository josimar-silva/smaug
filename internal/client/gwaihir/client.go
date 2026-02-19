package gwaihir

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

// maxResponseBodySize is the maximum allowed size for response bodies (1 MB).
const maxResponseBodySize = 1 * 1024 * 1024

// maxLogBodySize is the maximum number of bytes from a response body included in log output.
// This prevents secrets or large payloads from leaking into logs.
const maxLogBodySize = 200

// truncateBody returns a safe, length-limited string of body for use in log fields.
// It never exposes more than maxLogBodySize bytes so that credentials or other
// sensitive content that may appear in error response bodies are not written to logs.
func truncateBody(body []byte) string {
	if len(body) <= maxLogBodySize {
		return string(body)
	}
	return string(body[:maxLogBodySize]) + "… [truncated]"
}

// Log field names as constants to avoid duplication
const (
	logFieldURL        = "url"
	logFieldError      = "error"
	logFieldStatusCode = "status_code"
	logFieldMachineID  = "machine_id"
	logFieldMessage    = "message"
)

// Log messages as constants to avoid duplication
const (
	logMsgWoLRequestFailed    = "WoL request failed"
	logMsgWoLCommandSent      = "WoL command sent successfully"
	logMsgMarshalFailed       = "failed to marshal WoL request"
	logMsgCreateRequestFailed = "failed to create HTTP request"
	logMsgCloseBodyFailed     = "failed to close response body"
	logMsgReadBodyFailed      = "failed to read response body"
	logMsgAuthFailed          = "authentication failed"
	logMsgMachineNotFound     = "machine not found in allowlist"
	logMsgParseResponseFailed = "failed to parse success response"
	logMsgSendingWoL          = "sending WoL command via Gwaihir"
)

// Error message templates
const (
	errMsgMarshalRequest = "%w: failed to marshal request: %w"
	errMsgCreateRequest  = "%w: failed to create request for %s: %w"
	errMsgNetworkError   = "%w: %s: %w"
	errMsgReadBody       = "%w: failed to read response body: %w"
	errFmtStatusMessage  = "%w: %d: %s"
	errFmtStatus         = "%w: %d"
)

// Client is a REST client for the Gwaihir Wake-on-LAN service.
// It provides methods to send WoL commands via the Gwaihir HTTP API.
type Client struct {
	baseURL     string            // Base URL of the Gwaihir service
	apiKey      string            // API key for authentication
	timeout     time.Duration     // HTTP request timeout
	httpClient  *http.Client      // HTTP client for making requests
	logger      *logger.Logger    // Structured logger
	metrics     *metrics.Registry // Metrics registry (optional)
	retryConfig RetryConfig       // Retry configuration (defaults applied at construction)
}

// NewClient creates a new Gwaihir REST client with the given configuration.
//
// Parameters:
//   - config: Client configuration including base URL, API key, timeout, and retry
//     settings. RetryConfig must be set explicitly; use NewRetryConfig() for defaults.
//   - logger: Structured logger for client operations
//
// Returns:
//   - A pointer to the initialized Client
//   - An error if any required parameter is empty/nil or if timeout is zero
//
// Possible errors:
//   - ErrEmptyBaseURL if config.BaseURL is empty
//   - ErrEmptyAPIKey if config.APIKey is empty
//   - ErrInvalidTimeout if config.Timeout is zero or negative
//   - ErrNilLogger if logger is nil
//   - ErrInvalidRetryConfig if an explicit RetryConfig has MaxAttempts < 1 or BaseDelay <= 0
func NewClient(config ClientConfig, logger *logger.Logger) (*Client, error) {
	if err := config.RetryConfig.Validate(); err != nil {
		return nil, err
	}
	if err := validate(config, logger); err != nil {
		return nil, err
	}

	return &Client{
		baseURL: config.BaseURL,
		apiKey:  config.APIKey,
		timeout: config.Timeout,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		logger:      logger,
		retryConfig: config.RetryConfig,
	}, nil
}

func validate(config ClientConfig, log *logger.Logger) error {
	if config.BaseURL == "" {
		return ErrEmptyBaseURL
	}
	if _, err := url.Parse(config.BaseURL); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidBaseURL, err)
	}
	if config.APIKey == "" {
		return ErrEmptyAPIKey
	}
	if config.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	if log == nil {
		return ErrNilLogger
	}
	return nil
}

// SetMetrics sets the metrics registry for recording API call metrics.
// Should be called before the client is actively used.
func (c *Client) SetMetrics(reg *metrics.Registry) {
	c.metrics = reg
}

// SendWoL sends a Wake-on-LAN command to the specified machine via the Gwaihir service.
// When a transient error occurs (5xx response or network failure) the request is
// retried up to c.retryConfig.MaxAttempts times using exponential backoff with jitter.
// Permanent errors (4xx) are never retried.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - machineID: Target machine ID from Gwaihir's allowlist (e.g., "saruman")
//
// Returns:
//   - nil if the WoL packet was sent successfully (202 Accepted)
//   - ErrInvalidInput if machine ID is empty
//   - ErrAuthenticationFailed (wrapped) if the API key is invalid (401)
//   - ErrMachineNotFound (wrapped) if the machine is not in the allowlist (404)
//   - ErrWoLRequestFailed (wrapped) if the Gwaihir service returns an error (4xx, 5xx)
//   - ErrNetworkError (wrapped) if the HTTP request fails due to network issues
//
// Use errors.Is() to check for specific error types:
//
//	if errors.Is(err, ErrAuthenticationFailed) {
//	    // Handle authentication error
//	}
//	if errors.Is(err, ErrMachineNotFound) {
//	    // Handle machine not in allowlist
//	}
func (c *Client) SendWoL(ctx context.Context, machineID string) error {
	if machineID == "" {
		return ErrInvalidInput
	}

	c.logger.DebugContext(ctx, logMsgSendingWoL,
		logFieldMachineID, machineID,
	)

	startTime := time.Now()
	err := c.sendWithRetry(ctx, machineID)
	duration := time.Since(startTime).Seconds()

	// Record API call metric
	if c.metrics != nil {
		success := err == nil
		c.metrics.Gwaihir.RecordAPICall("send_wol", success, duration)
	}

	return err
}

func (c *Client) sendWithRetry(ctx context.Context, machineID string) error {
	wolReq := WoLRequest{MachineID: machineID}

	var (
		lastErr    error
		statusCode int
		respBody   []byte
	)

	for attempt := 1; attempt <= c.retryConfig.MaxAttempts; attempt++ {
		if attempt > 1 {
			backoff := c.retryConfig.Backoff(attempt - 1)
			c.logger.WarnContext(ctx, "retrying WoL request after transient error",
				logFieldMachineID, machineID,
				"attempt", attempt,
				"max_attempts", c.retryConfig.MaxAttempts,
				"backoff_ms", backoff.Milliseconds(),
				logFieldError, lastErr,
			)
			if sleepErr := sleepWithContext(ctx, backoff); sleepErr != nil {
				return fmt.Errorf("retry backoff interrupted: %w", sleepErr)
			}
		}

		var reqErr error
		statusCode, respBody, reqErr = c.executeWoLRequest(ctx, wolReq)
		if reqErr != nil {
			lastErr = reqErr
			c.logger.WarnContext(ctx, "WoL attempt failed with network error",
				logFieldMachineID, machineID,
				"attempt", attempt,
				"max_attempts", c.retryConfig.MaxAttempts,
				logFieldError, reqErr,
			)
			continue
		}

		if isSuccessStatus(statusCode) || isNonRetryableStatus(statusCode) {
			return c.handleWoLResponse(ctx, statusCode, respBody, machineID)
		}

		lastErr = fmt.Errorf(errFmtStatus, ErrWoLRequestFailed, statusCode)
		c.logger.WarnContext(ctx, "WoL attempt failed with server error",
			logFieldMachineID, machineID,
			"attempt", attempt,
			"max_attempts", c.retryConfig.MaxAttempts,
			logFieldStatusCode, statusCode,
		)
	}

	if lastErr != nil && statusCode == 0 {
		return lastErr
	}

	return c.handleWoLResponse(ctx, statusCode, respBody, machineID)
}

func isSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

func isNonRetryableStatus(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) executeWoLRequest(ctx context.Context, req WoLRequest) (int, []byte, error) {
	body, err := c.marshalWoLRequest(ctx, req)
	if err != nil {
		return 0, nil, err
	}

	url := c.buildWoLURL()
	httpReq, err := c.createHTTPRequest(ctx, url, body)
	if err != nil {
		return 0, nil, err
	}

	c.setRequestHeaders(httpReq)

	statusCode, respBody, err := c.sendHTTPRequestAndReadResponse(ctx, url, httpReq)
	if err != nil {
		return statusCode, nil, err
	}

	return statusCode, respBody, nil
}

func (c *Client) marshalWoLRequest(ctx context.Context, req WoLRequest) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		c.logger.ErrorContext(ctx, logMsgMarshalFailed,
			logFieldError, err,
		)
		return nil, fmt.Errorf(errMsgMarshalRequest, ErrInternalError, err)
	}
	return body, nil
}

func (c *Client) buildWoLURL() string {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		// This should never happen: baseURL is validated with url.Parse during construction.
		// Log an error so the failure is visible instead of being silently masked.
		c.logger.Error("failed to parse baseURL; this indicates a bug — baseURL must be validated before use",
			logFieldURL, c.baseURL,
			logFieldError, err,
		)
		return c.baseURL + "/wol"
	}

	wolPath := &url.URL{Path: "/wol"}
	return base.ResolveReference(wolPath).String()
}

func (c *Client) createHTTPRequest(ctx context.Context, url string, body []byte) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		c.logger.ErrorContext(ctx, logMsgCreateRequestFailed,
			logFieldURL, url,
			logFieldError, err,
		)
		return nil, fmt.Errorf(errMsgCreateRequest, ErrNetworkError, url, err)
	}
	return httpReq, nil
}

func (c *Client) setRequestHeaders(httpReq *http.Request) {
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
}

func (c *Client) sendHTTPRequestAndReadResponse(ctx context.Context, url string, httpReq *http.Request) (int, []byte, error) {
	resp, err := c.executeHTTPRequest(ctx, url, httpReq) //nolint:bodyclose // Body is closed in defer below
	if err != nil {
		return 0, nil, err
	}
	defer c.closeResponseBody(ctx, url, resp.Body)

	respBody, err := c.readResponseBody(ctx, url, resp.StatusCode, resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}

	return resp.StatusCode, respBody, nil
}

func (c *Client) executeHTTPRequest(ctx context.Context, url string, httpReq *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		c.logger.WarnContext(ctx, logMsgWoLRequestFailed,
			logFieldURL, url,
			logFieldError, err,
		)
		return nil, fmt.Errorf(errMsgNetworkError, ErrNetworkError, url, err)
	}
	return resp, nil
}

func (c *Client) closeResponseBody(ctx context.Context, url string, body io.ReadCloser) {
	if closeErr := body.Close(); closeErr != nil {
		c.logger.WarnContext(ctx, logMsgCloseBodyFailed,
			logFieldURL, url,
			logFieldError, closeErr,
		)
	}
}

func (c *Client) readResponseBody(ctx context.Context, url string, statusCode int, body io.Reader) ([]byte, error) {
	limitedReader := io.LimitReader(body, maxResponseBodySize)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		c.logger.WarnContext(ctx, logMsgReadBodyFailed,
			logFieldURL, url,
			logFieldStatusCode, statusCode,
			logFieldError, err,
		)
		return nil, fmt.Errorf(errMsgReadBody, ErrNetworkError, err)
	}

	// Check if response was truncated (hit the size limit)
	if int64(len(respBody)) == maxResponseBodySize {
		c.logger.WarnContext(ctx, "response body may have been truncated",
			logFieldURL, url,
			logFieldStatusCode, statusCode,
			"max_size", maxResponseBodySize,
		)
	}

	return respBody, nil
}

func (c *Client) handleWoLResponse(ctx context.Context, statusCode int, respBody []byte, machineID string) error {
	if statusCode == http.StatusUnauthorized {
		return c.handleAuthenticationError(ctx, statusCode, respBody)
	}

	if statusCode == http.StatusNotFound {
		return c.handleMachineNotFoundError(ctx, statusCode, respBody, machineID)
	}

	if statusCode < 200 || statusCode >= 300 {
		return c.handleOtherErrorStatus(ctx, statusCode, respBody, machineID)
	}

	return c.handleSuccessResponse(ctx, statusCode, respBody, machineID)
}

func (c *Client) handleAuthenticationError(ctx context.Context, statusCode int, respBody []byte) error {
	var wolResp WoLResponse
	if err := json.Unmarshal(respBody, &wolResp); err == nil {
		c.logger.ErrorContext(ctx, logMsgAuthFailed,
			logFieldStatusCode, statusCode,
			logFieldMessage, wolResp.Message,
		)
		return fmt.Errorf(errFmtStatusMessage, ErrAuthenticationFailed, statusCode, wolResp.Message)
	} else {
		c.logger.WarnContext(ctx, logMsgParseResponseFailed,
			logFieldStatusCode, statusCode,
			logFieldError, err,
			"response_body", truncateBody(respBody),
		)
	}
	return fmt.Errorf(errFmtStatus, ErrAuthenticationFailed, statusCode)
}

func (c *Client) handleMachineNotFoundError(ctx context.Context, statusCode int, respBody []byte, machineID string) error {
	var wolResp WoLResponse
	if err := json.Unmarshal(respBody, &wolResp); err == nil {
		c.logger.WarnContext(ctx, logMsgMachineNotFound,
			logFieldMachineID, machineID,
			logFieldStatusCode, statusCode,
			logFieldMessage, wolResp.Message,
		)
		return fmt.Errorf(errFmtStatusMessage, ErrMachineNotFound, statusCode, wolResp.Message)
	} else {
		c.logger.WarnContext(ctx, logMsgParseResponseFailed,
			logFieldMachineID, machineID,
			logFieldStatusCode, statusCode,
			logFieldError, err,
			"response_body", truncateBody(respBody),
		)
	}
	return fmt.Errorf(errFmtStatus, ErrMachineNotFound, statusCode)
}

func (c *Client) handleOtherErrorStatus(ctx context.Context, statusCode int, respBody []byte, machineID string) error {
	var wolResp WoLResponse
	if json.Unmarshal(respBody, &wolResp) == nil {
		c.logger.WarnContext(ctx, logMsgWoLRequestFailed,
			logFieldMachineID, machineID,
			logFieldStatusCode, statusCode,
			logFieldMessage, wolResp.Message,
		)
		return fmt.Errorf(errFmtStatusMessage, ErrWoLRequestFailed, statusCode, wolResp.Message)
	}
	c.logger.WarnContext(ctx, logMsgWoLRequestFailed,
		logFieldMachineID, machineID,
		logFieldStatusCode, statusCode,
	)
	return fmt.Errorf(errFmtStatus, ErrWoLRequestFailed, statusCode)
}

func (c *Client) handleSuccessResponse(ctx context.Context, statusCode int, respBody []byte, machineID string) error {
	var wolResp WoLResponse
	if err := json.Unmarshal(respBody, &wolResp); err != nil {
		c.logger.WarnContext(ctx, logMsgParseResponseFailed,
			logFieldMachineID, machineID,
			logFieldStatusCode, statusCode,
			logFieldError, err,
		)
		c.logger.InfoContext(ctx, logMsgWoLCommandSent,
			logFieldMachineID, machineID,
			logFieldStatusCode, statusCode,
		)
		return nil
	}

	c.logger.InfoContext(ctx, logMsgWoLCommandSent,
		logFieldMachineID, machineID,
		logFieldStatusCode, statusCode,
		logFieldMessage, wolResp.Message,
	)

	return nil
}
