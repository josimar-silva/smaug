package proxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/store"
)

func TestNewRouteManager_ValidParameters(t *testing.T) {
	// Given: Valid dependencies
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()

	// When: Creating RouteManager
	rm, err := NewRouteManager(cfg, log, mw, store)

	// Then: RouteManager should be created successfully
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if rm == nil {
		t.Fatal("expected RouteManager to be created, got nil")
	}
	if rm.config != cfg {
		t.Error("expected config to be set")
	}
	if rm.logger == nil {
		t.Error("expected logger to be set")
	}
	if rm.shutdownTimeout != 30*time.Second {
		t.Errorf("expected default shutdown timeout to be 30s, got %v", rm.shutdownTimeout)
	}
}

func TestNewRouteManager_NilConfig(t *testing.T) {
	// Given: Nil config
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil config
	rm, err := NewRouteManager(nil, log, mw, store)

	// Then: Should return ErrConfigMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrConfigMissing) {
		t.Errorf("expected ErrConfigMissing, got: %v", err)
	}
}

func TestNewRouteManager_NilLogger(t *testing.T) {
	// Given: Nil logger
	cfg := &config.Config{}
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil logger
	rm, err := NewRouteManager(cfg, nil, mw, store)

	// Then: Should return ErrNilLogger
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrLoggerMissing) {
		t.Errorf("expected ErrNilLogger, got: %v", err)
	}
}

func TestNewRouteManager_NilMiddleware(t *testing.T) {
	// Given: Nil middleware
	cfg := &config.Config{}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	store := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil middleware
	rm, err := NewRouteManager(cfg, log, nil, store)

	// Then: Should return ErrNilMiddleware
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrMiddlewareMissing) {
		t.Errorf("expected ErrNilMiddleware, got: %v", err)
	}
}

func TestNewRouteManager_NilHealthStore(t *testing.T) {
	// Given: Nil health store
	cfg := &config.Config{}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()

	// When: Creating RouteManager with nil health store
	rm, err := NewRouteManager(cfg, log, mw, nil)

	// Then: Should return ErrHealthStoreMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrHealthStoreMissing) {
		t.Errorf("expected ErrHealthStoreMissing, got: %v", err)
	}
}

func TestListenerState_String(t *testing.T) {
	tests := []struct {
		name     string
		state    listenerState
		expected string
	}{
		{
			name:     "initial state",
			state:    stateInitial,
			expected: "initial",
		},
		{
			name:     "starting state",
			state:    stateStarting,
			expected: "starting",
		},
		{
			name:     "running state",
			state:    stateRunning,
			expected: "running",
		},
		{
			name:     "stopping state",
			state:    stateStopping,
			expected: "stopping",
		},
		{
			name:     "stopped state",
			state:    stateStopped,
			expected: "stopped",
		},
		{
			name:     "failed state",
			state:    stateFailed,
			expected: "failed",
		},
		{
			name:     "unknown state",
			state:    listenerState(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: Converting state to string
			result := tt.state.String()

			// Then: Should match expected value
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteManager_GetActiveRoutes_EmptyManager(t *testing.T) {
	// Given: RouteManager with no routes
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	rm, err := NewRouteManager(cfg, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// When: Getting active routes
	routes := rm.GetActiveRoutes()

	// Then: Should return empty slice
	if len(routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(routes))
	}
}

func TestRouteManager_Start_AlreadyRunning(t *testing.T) {
	// Given: A running RouteManager
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	rm, err := NewRouteManager(cfg, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		if err := rm.Stop(); err != nil {
			t.Logf("failed to stop route manager: %v", err)
		}
	}()

	// When: Starting again
	err = rm.Start(ctx)

	// Then: Should return ErrAlreadyRunning
	if err == nil {
		t.Error("expected error when starting already running manager")
	}
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("expected ErrAlreadyRunning, got: %v", err)
	}
}

func TestRouteManager_Stop_NotRunning(t *testing.T) {
	// Given: A RouteManager that was never started
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	rm, err := NewRouteManager(cfg, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// When: Stopping
	err = rm.Stop()

	// Then: Should return ErrNotRunning
	if err == nil {
		t.Error("expected error when stopping non-running manager")
	}
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("expected ErrNotRunning, got: %v", err)
	}
}
