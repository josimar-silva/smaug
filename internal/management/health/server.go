package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

var (
	// ErrServerAlreadyRunning is returned when Start is called on an already running server.
	ErrServerAlreadyRunning = errors.New("management server is already running")

	// ErrServerNotRunning is returned when Stop is called on a non-running server.
	ErrServerNotRunning = errors.New("management server is not running")

	// ErrServerShutdownTimeout is returned when graceful shutdown exceeds the timeout.
	ErrServerShutdownTimeout = errors.New("server shutdown timeout exceeded")
)

const (
	defaultShutdownTimeout = 10 * time.Second
)

// Server represents the management HTTP server that serves health check and version endpoints.
type Server struct {
	port            int
	routeProvider   RouteStatusProvider
	versionInfo     VersionInfo
	logger          *logger.Logger
	server          *http.Server
	startTime       time.Time
	mu              sync.RWMutex
	running         bool
	shutdownTimeout time.Duration
	started         chan struct{}
	startedOnce     sync.Once
	startErr        error
}

// NewServer creates a new management server that will listen on the specified port.
func NewServer(port int, provider RouteStatusProvider, versionInfo VersionInfo, log *logger.Logger) *Server {
	return &Server{
		port:            port,
		routeProvider:   provider,
		versionInfo:     versionInfo,
		logger:          log,
		shutdownTimeout: defaultShutdownTimeout,
		started:         make(chan struct{}),
	}
}

// Start starts the management HTTP server in a background goroutine.
// Returns an error if the server is already running or fails to bind immediately.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrServerAlreadyRunning
	}
	s.mu.Unlock()

	s.startTime = time.Now()

	mux := http.NewServeMux()
	mux.Handle("/health", NewHealthHandler(s.routeProvider, s.versionInfo, s.logger, s.startTime))
	mux.Handle("/live", NewLiveHandler(s.logger))
	mux.Handle("/ready", NewReadyHandler(s.routeProvider, s.logger))
	mux.Handle("/version", NewVersionHandler(s.versionInfo, s.logger))

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.port),
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		s.mu.RLock()
		running := s.running
		s.mu.RUnlock()

		if running {
			s.startedOnce.Do(func() { close(s.started) })
		}
	}()

	// Run server in background goroutine
	go func() {
		s.logger.Info("starting management server", "port", s.port)

		s.mu.Lock()
		s.running = true
		s.mu.Unlock()

		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.running = false
			s.startErr = err
			s.mu.Unlock()

			s.logger.Error("management server error", "error", err)
			s.startedOnce.Do(func() { close(s.started) })
		}
	}()

	go func() {
		<-ctx.Done()
		// Context cancelled, shutdown the server
		s.mu.RLock()
		if s.running {
			s.mu.RUnlock()
			s.logger.Info("context cancelled, shutting down management server", "port", s.port)
			shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
			defer cancel()
			if err := s.server.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("error during context-triggered shutdown", "error", err)
			}
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
		} else {
			s.mu.RUnlock()
		}
	}()

	// Wait for startup to complete (success or failure)
	<-s.started

	s.mu.RLock()
	startErr := s.startErr
	s.mu.RUnlock()

	return startErr
}

// Stop gracefully shuts down the management HTTP server.
// Returns an error if the server is not running or shutdown times out.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return ErrServerNotRunning
	}

	s.logger.Info("stopping management server", "port", s.port)

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrServerShutdownTimeout, err)
		}

		return fmt.Errorf("server shutdown: %w", err)
	}

	s.running = false
	s.logger.Info("management server stopped", "port", s.port)

	return nil
}
