package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

const (
	// HeaderXForwardedFor identifies the originating IP address of a client connecting through a proxy
	HeaderXForwardedFor = "X-Forwarded-For"
	// HeaderXForwardedProto identifies the protocol (HTTP or HTTPS) that a client used to connect
	HeaderXForwardedProto = "X-Forwarded-Proto"
	// HeaderXForwardedHost identifies the original host requested by the client
	HeaderXForwardedHost = "X-Forwarded-Host"
)

// ActivityRecorder records that activity has been seen for a given route.
// Implementations must be safe for concurrent use by multiple goroutines.
// The RecordActivity call must not perform I/O or block for an unbounded duration.
type ActivityRecorder interface {
	RecordActivity(routeID string)
}

// ProxyHandlerOption is a functional option for configuring a ProxyHandler.
type ProxyHandlerOption func(*proxyHandlerOptions)

// proxyHandlerOptions holds the optional configuration for NewProxyHandler.
type proxyHandlerOptions struct {
	activityRecorder ActivityRecorder
	routeID          string
}

// WithActivityRecorder attaches an ActivityRecorder to the proxy handler.
// On every request — including requests that result in an error — RecordActivity
// is called with routeID before the request is forwarded to the backend.
// The call is synchronous but fast (in-memory only); it never performs I/O.
func WithActivityRecorder(recorder ActivityRecorder, routeID string) ProxyHandlerOption {
	return func(o *proxyHandlerOptions) {
		o.activityRecorder = recorder
		o.routeID = routeID
	}
}

// ProxyHandler implements an HTTP reverse proxy handler.
type ProxyHandler struct {
	proxy            *httputil.ReverseProxy
	log              *logger.Logger
	target           *url.URL
	activityRecorder ActivityRecorder
	routeID          string
}

// NewProxyHandler creates a new reverse proxy handler that forwards requests to the target URL.
// It preserves request headers including X-Forwarded-* headers and returns 502 Bad Gateway
// if the backend is unreachable.
//
// Parameters:
//   - targetURL: The backend URL to proxy requests to
//   - log: The logger instance for logging proxy operations and errors
//   - opts: Optional functional options (e.g. WithActivityRecorder)
func NewProxyHandler(targetURL string, log *logger.Logger, opts ...ProxyHandlerOption) http.Handler {
	options := &proxyHandlerOptions{}
	for _, opt := range opts {
		opt(options)
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		return newErrorHandler(options, func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL", "url", targetURL, "error", err)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Scheme == "" {
		return newErrorHandler(options, func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: missing scheme", "url", targetURL)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Scheme != "http" && target.Scheme != "https" {
		return newErrorHandler(options, func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: unsupported scheme", "url", targetURL, "scheme", target.Scheme)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Host == "" {
		return newErrorHandler(options, func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: missing host", "url", targetURL)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	ph := &ProxyHandler{
		proxy:            proxy,
		log:              log,
		target:           target,
		activityRecorder: options.activityRecorder,
		routeID:          options.routeID,
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		ph.log.Error("proxy error",
			"backend", target.String(),
			"path", r.URL.Path,
			"method", r.Method,
			"error", err,
		)
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
	}

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		originalDirector(req)
		addForwardedHeaders(req, originalHost)
	}

	return ph
}

// ServeHTTP implements the http.Handler interface.
// It records activity before forwarding the request to the backend.
func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ph.activityRecorder != nil {
		ph.activityRecorder.RecordActivity(ph.routeID)
	}
	ph.proxy.ServeHTTP(w, r)
}

// newErrorHandler wraps an error response function with an optional activity recording
// call. This ensures RecordActivity is invoked even when the proxy handler detects an
// invalid target URL before any proxying occurs.
func newErrorHandler(opts *proxyHandlerOptions, errFn http.HandlerFunc) http.Handler {
	if opts.activityRecorder == nil {
		return errFn
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts.activityRecorder.RecordActivity(opts.routeID)
		errFn(w, r)
	})
}

// addForwardedHeaders adds X-Forwarded-Proto and X-Forwarded-Host headers to preserve client information.
// X-Forwarded-For is handled automatically by ReverseProxy.
// These headers are used by backend services to understand the original request.
//
// Parameters:
//   - r: The HTTP request
//   - originalHost: The original Host header value captured before Director runs
func addForwardedHeaders(r *http.Request, originalHost string) {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	if r.Header.Get(HeaderXForwardedProto) == "" {
		r.Header.Set(HeaderXForwardedProto, proto)
	}

	if r.Header.Get(HeaderXForwardedHost) == "" {
		r.Header.Set(HeaderXForwardedHost, originalHost)
	}
}
