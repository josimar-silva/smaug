package metrics

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

var (
	// ErrServerAlreadyRunning is returned when Start is called on an already running server.
	ErrServerAlreadyRunning = errors.New("metrics server is already running")

	// ErrServerNotRunning is returned when Stop is called on a non-running server.
	ErrServerNotRunning = errors.New("metrics server is not running")

	// ErrServerShutdownTimeout is returned when graceful shutdown exceeds the timeout.
	ErrServerShutdownTimeout = errors.New("server shutdown timeout exceeded")
)

const (
	defaultShutdownTimeout = 10 * time.Second
)

// Server represents the metrics HTTP server that serves Prometheus metrics.
type Server struct {
	port            int
	registry        *metrics.Registry
	logger          *logger.Logger
	server          *http.Server
	mu              sync.RWMutex
	running         bool
	shutdownTimeout time.Duration
	started         chan struct{}
	startedOnce     sync.Once
	startErr        error
}

// NewServer creates a new metrics server that will listen on the specified port.
func NewServer(port int, registry *metrics.Registry, log *logger.Logger) *Server {
	return &Server{
		port:            port,
		registry:        registry,
		logger:          log,
		shutdownTimeout: defaultShutdownTimeout,
		started:         make(chan struct{}),
	}
}

// Start starts the metrics HTTP server in a background goroutine.
// Returns an error if the server is already running or fails to bind immediately.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrServerAlreadyRunning
	}
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.Handle("/metrics", s.registry.Handler())

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
		s.logger.Info("starting metrics server", "port", s.port)

		s.mu.Lock()
		s.running = true
		s.mu.Unlock()

		if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.mu.Lock()
			s.running = false
			s.startErr = err
			s.mu.Unlock()

			s.logger.Error("metrics server error", "error", err)
			s.startedOnce.Do(func() { close(s.started) })
		}
	}()

	go func() {
		<-ctx.Done()
		// Context cancelled, shutdown the server
		s.mu.RLock()
		if s.running {
			s.mu.RUnlock()
			s.logger.Info("context cancelled, shutting down metrics server", "port", s.port)
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
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

// IsRunning returns whether the metrics server is currently running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Stop gracefully shuts down the metrics HTTP server.
// Returns an error if the server is not running or shutdown times out.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return ErrServerNotRunning
	}

	s.logger.Info("stopping metrics server", "port", s.port)

	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: %v", ErrServerShutdownTimeout, err)
		}

		return fmt.Errorf("server shutdown: %w", err)
	}

	s.running = false
	s.logger.Info("metrics server stopped", "port", s.port)

	return nil
}
