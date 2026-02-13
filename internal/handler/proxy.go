package handler

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

// ProxyHandler implements an HTTP reverse proxy handler.
type ProxyHandler struct {
	proxy  *httputil.ReverseProxy
	log    *logger.Logger
	target *url.URL
}

// NewProxyHandler creates a new reverse proxy handler that forwards requests to the target URL.
// It preserves request headers including X-Forwarded-* headers and returns 502 Bad Gateway
// if the backend is unreachable.
//
// Parameters:
//   - targetURL: The backend URL to proxy requests to
//   - log: The logger instance for logging proxy operations and errors
func NewProxyHandler(targetURL string, log *logger.Logger) http.Handler {
	target, err := url.Parse(targetURL)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL", "url", targetURL, "error", err)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Scheme == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: missing scheme", "url", targetURL)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Scheme != "http" && target.Scheme != "https" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: unsupported scheme", "url", targetURL, "scheme", target.Scheme)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	if target.Host == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Error("invalid target URL: missing host", "url", targetURL)
			http.Error(w, "Invalid target URL", http.StatusBadGateway)
		})
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	ph := &ProxyHandler{
		proxy:  proxy,
		log:    log,
		target: target,
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
func (ph *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ph.proxy.ServeHTTP(w, r)
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
