package proxy

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/store"
)

func createTestConfigManager(t *testing.T, cfg *config.Config) *config.ConfigManager {
	t.Helper()

	ensureServersExist(cfg)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}

	t.Cleanup(func() {
		_ = configMgr.Stop()
	})

	return configMgr
}

func ensureServersExist(cfg *config.Config) {
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]config.Server)
	}

	for _, route := range cfg.Routes {
		if _, exists := cfg.Servers[route.Server]; !exists {
			cfg.Servers[route.Server] = config.Server{}
		}
	}
}

func TestNewRouteManagerValidParameters(t *testing.T) {
	// Given: Valid dependencies
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	// When: Creating RouteManager
	rm, err := NewRouteManager(configMgr, log, mw, healthStore)

	// Then: RouteManager should be created successfully
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if rm == nil {
		t.Fatal("expected RouteManager to be created, got nil")
	}
	if rm.configMgr != configMgr {
		t.Error("expected config manager to be set")
	}
	if rm.logger == nil {
		t.Error("expected logger to be set")
	}
	if rm.shutdownTimeout != 30*time.Second {
		t.Errorf("expected default shutdown timeout to be 30s, got %v", rm.shutdownTimeout)
	}
}

func TestNewRouteManagerNilConfigManager(t *testing.T) {
	// Given: Nil config manager
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil config manager
	rm, err := NewRouteManager(nil, log, mw, healthStore)

	// Then: Should return ErrConfigManagerMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrConfigManagerMissing) {
		t.Errorf("expected ErrConfigManagerMissing, got: %v", err)
	}
}

func TestNewRouteManagerNilLogger(t *testing.T) {
	// Given: Nil logger
	cfg := &config.Config{}
	configMgr := createTestConfigManager(t, cfg)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil logger
	rm, err := NewRouteManager(configMgr, nil, mw, healthStore)

	// Then: Should return ErrLoggerMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrLoggerMissing) {
		t.Errorf("expected ErrLoggerMissing, got: %v", err)
	}
}

func TestNewRouteManagerNilMiddleware(t *testing.T) {
	// Given: Nil middleware
	cfg := &config.Config{}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	healthStore := store.NewInMemoryHealthStore()

	// When: Creating RouteManager with nil middleware
	rm, err := NewRouteManager(configMgr, log, nil, healthStore)

	// Then: Should return ErrMiddlewareMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrMiddlewareMissing) {
		t.Errorf("expected ErrMiddlewareMissing, got: %v", err)
	}
}

func TestNewRouteManagerNilHealthStore(t *testing.T) {
	// Given: Nil health store
	cfg := &config.Config{}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()

	// When: Creating RouteManager with nil health store
	rm, err := NewRouteManager(configMgr, log, mw, nil)

	// Then: Should return ErrHealthStoreMissing
	if rm != nil {
		t.Error("expected nil RouteManager")
	}
	if !errors.Is(err, ErrHealthStoreMissing) {
		t.Errorf("expected ErrHealthStoreMissing, got: %v", err)
	}
}

func TestListenerStateString(t *testing.T) {
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

func TestRouteManagerGetActiveRoutesEmptyManager(t *testing.T) {
	// Given: RouteManager with no routes
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
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

func TestRouteManagerStartAlreadyRunning(t *testing.T) {
	// Given: A running RouteManager
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
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

func TestRouteManagerStopNotRunning(t *testing.T) {
	// Given: A RouteManager that was never started
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
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

func TestRouteManagerStartSingleRoute(t *testing.T) {
	// Given: RouteManager with one route
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18080,
				Upstream: "http://localhost:19000",
				Server:   "test-server",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// When: Starting route manager
	ctx := context.Background()
	err = rm.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		if err := rm.Stop(); err != nil {
			t.Errorf("failed to stop route manager: %v", err)
		}
	}()

	// Give listeners time to start
	time.Sleep(50 * time.Millisecond)

	// Then: Route should be active
	routes := rm.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Name != "test" {
		t.Errorf("expected route name 'test', got %q", routes[0].Name)
	}
	if routes[0].Port != 18080 {
		t.Errorf("expected port 18080, got %d", routes[0].Port)
	}
	if routes[0].State != "starting" && routes[0].State != "running" {
		t.Errorf("expected state 'starting' or 'running', got %q", routes[0].State)
	}
}

func TestRouteManagerStartMultipleRoutes(t *testing.T) {
	// Given: RouteManager with multiple routes
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "route1",
				Listen:   18081,
				Upstream: "http://localhost:19001",
				Server:   "server1",
			},
			{
				Name:     "route2",
				Listen:   18082,
				Upstream: "http://localhost:19002",
				Server:   "server2",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// When: Starting route manager
	ctx := context.Background()
	err = rm.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		if err := rm.Stop(); err != nil {
			t.Errorf("failed to stop route manager: %v", err)
		}
	}()

	// Then: Both routes should be active
	routes := rm.GetActiveRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
}

func TestRouteManagerStopGraceful(t *testing.T) {
	// Given: A running RouteManager with one route
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18083,
				Upstream: "http://localhost:19003",
				Server:   "test-server",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}

	// When: Stopping gracefully
	err = rm.Stop()

	// Then: Should stop without error
	if err != nil {
		t.Errorf("expected graceful stop, got error: %v", err)
	}

	// And: Should not be able to call Stop again
	err = rm.Stop()
	if !errors.Is(err, ErrNotRunning) {
		t.Errorf("expected ErrNotRunning after stop, got: %v", err)
	}
}

func TestRouteManagerStopRouteSuccess(t *testing.T) {
	// Given: A running RouteManager with one route
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18090,
				Upstream: "http://localhost:19010",
				Server:   "test-server",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	time.Sleep(50 * time.Millisecond)

	// When: Stopping specific route
	err = rm.stopRoute("test:18090")

	// Then: Should stop successfully
	if err != nil {
		t.Errorf("expected successful stop, got error: %v", err)
	}
}

func TestRouteManagerStopRouteNotFound(t *testing.T) {
	// Given: A running RouteManager
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18091,
				Upstream: "http://localhost:19011",
				Server:   "test-server",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	// When: Stopping non-existent route
	err = rm.stopRoute("nonexistent:9999")

	// Then: Should return error
	if err == nil {
		t.Error("expected error for non-existent route")
	}
}

func TestRouteManagerStartRouteSuccess(t *testing.T) {
	// Given: A running RouteManager
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	// When: Starting a new route
	newRoute := config.Route{
		Name:     "dynamic",
		Listen:   18092,
		Upstream: "http://localhost:19012",
		Server:   "test-server",
	}
	err = rm.startRoute(newRoute)

	// Then: Route should start successfully
	if err != nil {
		t.Errorf("expected successful start, got error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	routes := rm.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Name != "dynamic" {
		t.Errorf("expected route name 'dynamic', got %q", routes[0].Name)
	}
}

func TestRouteManagerCleanupStoppedRoutes(t *testing.T) {
	// Given: A RouteManager with multiple routes
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "route1",
				Listen:   18093,
				Upstream: "http://localhost:19013",
				Server:   "server1",
			},
			{
				Name:     "route2",
				Listen:   18094,
				Upstream: "http://localhost:19014",
				Server:   "server2",
			},
			{
				Name:     "route3",
				Listen:   18095,
				Upstream: "http://localhost:19015",
				Server:   "server3",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	store := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, store)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	time.Sleep(50 * time.Millisecond)

	// Stop the routes first before cleanup
	if err := rm.stopRoute("route1:18093"); err != nil {
		t.Fatalf("failed to stop route1: %v", err)
	}
	if err := rm.stopRoute("route3:18095"); err != nil {
		t.Fatalf("failed to stop route3: %v", err)
	}

	// When: Cleaning up specific routes
	keysToCleanup := []string{"route1:18093", "route3:18095"}
	rm.cleanupStoppedRoutes(keysToCleanup)

	// Then: Only route2 should remain
	rm.mu.RLock()
	routeCount := len(rm.routes)
	mapCount := len(rm.routeMap)
	rm.mu.RUnlock()

	if routeCount != 1 {
		t.Errorf("expected 1 route in slice, got %d", routeCount)
	}
	if mapCount != 1 {
		t.Errorf("expected 1 route in map, got %d", mapCount)
	}

	routes := rm.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 active route, got %d", len(routes))
	}
	if routes[0].Name != "route2" {
		t.Errorf("expected remaining route 'route2', got %q", routes[0].Name)
	}
}

func TestApplyRouteChangesSuccessfulReload(t *testing.T) {
	// Given: RouteManager with multiple routes and change detection
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "route1",
				Listen:   18100,
				Upstream: "http://localhost:19100",
				Server:   "server1",
			},
			{
				Name:     "route2",
				Listen:   18101,
				Upstream: "http://localhost:19101",
				Server:   "server2",
			},
			{
				Name:     "route3",
				Listen:   18102,
				Upstream: "http://localhost:19102",
				Server:   "server3",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	// When: Applying route changes (remove route2, modify route1, keep route3)
	changes := RouteChanges{
		Removed: []string{"route2:18101"},
		Modified: []config.Route{
			{
				Name:     "route1",
				Listen:   18100,
				Upstream: "http://localhost:19100",
				Server:   "server1",
			},
		},
		Added:     []config.Route{},
		Unchanged: []string{"route3:18102"},
	}

	err = rm.applyRouteChanges(changes)

	// Then: Should apply changes successfully
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	routes := rm.GetActiveRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes after changes, got %d", len(routes))
	}

	routeMap := make(map[string]bool)
	for _, r := range routes {
		routeMap[r.Name] = true
	}

	if !routeMap["route1"] || !routeMap["route3"] {
		t.Error("expected route1 and route3 to be active")
	}
}

func TestApplyRouteChangesWithAdditions(t *testing.T) {
	// Given: Running RouteManager with no routes
	cfg := &config.Config{
		Routes: []config.Route{},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	// When: Adding new routes
	changes := RouteChanges{
		Added: []config.Route{
			{
				Name:     "new1",
				Listen:   18110,
				Upstream: "http://localhost:19110",
				Server:   "server1",
			},
			{
				Name:     "new2",
				Listen:   18111,
				Upstream: "http://localhost:19111",
				Server:   "server2",
			},
		},
		Removed:   []string{},
		Modified:  []config.Route{},
		Unchanged: []string{},
	}

	err = rm.applyRouteChanges(changes)

	// Then: Should add routes successfully
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	routes := rm.GetActiveRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
}

func TestHandleReloadNoChanges(t *testing.T) {
	// Given: RouteManager with a route that has no configuration changes
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18120,
				Upstream: "http://localhost:19120",
				Server:   "server1",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	if err := rm.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer func() {
		_ = rm.Stop()
	}()

	// When: Handling reload with identical config
	err = rm.handleReload()

	// Then: Should succeed without making changes
	if err != nil {
		t.Errorf("expected no error on reload, got: %v", err)
	}

	// Routes should remain unchanged
	routes := rm.GetActiveRoutes()
	if len(routes) != 1 {
		t.Errorf("expected 1 route to remain, got %d", len(routes))
	}
}

func TestRunListenerStartupFailure(t *testing.T) {
	// Given: Route configured to listen on a port that's already in use
	cfg := &config.Config{
		Routes: []config.Route{
			{
				Name:     "test",
				Listen:   18130,
				Upstream: "http://localhost:19130",
				Server:   "server1",
			},
		},
	}
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()
	configMgr := createTestConfigManager(t, cfg)
	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()

	// Block the port first
	blockingServer := &http.Server{
		Addr:              ":18130",
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	blockingDone := make(chan struct{})
	go func() {
		_ = blockingServer.ListenAndServe()
		close(blockingDone)
	}()

	t.Cleanup(func() {
		_ = blockingServer.Close()
		select {
		case <-blockingDone:
		case <-time.After(time.Second):
		}
	})

	// Give blocking server time to start
	time.Sleep(50 * time.Millisecond)

	// When: Attempting to start route on already-bound port
	_ = rm.Start(ctx)

	// Give listener goroutine time to fail
	time.Sleep(200 * time.Millisecond)

	// Then: Route should be in failed state
	routes := rm.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	failedRoute := routes[0]
	if failedRoute.State != "failed" {
		t.Errorf("expected route to be in 'failed' state, got '%s'", failedRoute.State)
	}

	_ = rm.Stop()
}

func TestGetActiveRouteCountReturnsZeroWhenNotStarted(t *testing.T) {
	// Given: A route manager that has not been started
	cfg := &config.Config{
		Routes: []config.Route{
			{Name: "test1", Listen: 18201, Upstream: "http://localhost:8001", Server: "server1"},
			{Name: "test2", Listen: 18202, Upstream: "http://localhost:8002", Server: "server2"},
			{Name: "test3", Listen: 18203, Upstream: "http://localhost:8003", Server: "server3"},
		},
	}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// When: Getting active route count before start
	count := rm.GetActiveRouteCount()

	// Then: Count should be 0
	if count != 0 {
		t.Errorf("expected 0 active routes, got %d", count)
	}
}

func TestGetActiveRouteCountReturnsCorrectCountWhenRunning(t *testing.T) {
	// Given: A route manager with 3 routes started
	cfg := &config.Config{
		Routes: []config.Route{
			{Name: "test1", Listen: 18211, Upstream: "http://localhost:8011", Server: "server1"},
			{Name: "test2", Listen: 18212, Upstream: "http://localhost:8012", Server: "server2"},
			{Name: "test3", Listen: 18213, Upstream: "http://localhost:8013", Server: "server3"},
		},
	}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	err = rm.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}

	t.Cleanup(func() {
		_ = rm.Stop()
	})

	// Give routes time to start and reach running state
	time.Sleep(250 * time.Millisecond)

	// When: Getting active route count
	count := rm.GetActiveRouteCount()

	// Then: Count should be 3
	if count != 3 {
		t.Errorf("expected 3 active routes, got %d", count)
	}
}

func TestGetActiveRouteCountReturnsZeroAfterStop(t *testing.T) {
	// Given: A route manager that was started and then stopped
	cfg := &config.Config{
		Routes: []config.Route{
			{Name: "test1", Listen: 18221, Upstream: "http://localhost:8021", Server: "server1"},
			{Name: "test2", Listen: 18222, Upstream: "http://localhost:8022", Server: "server2"},
		},
	}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	ctx := context.Background()
	err = rm.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}

	// Give routes time to start
	time.Sleep(100 * time.Millisecond)

	// Stop the route manager
	err = rm.Stop()
	if err != nil {
		t.Fatalf("failed to stop route manager: %v", err)
	}

	// When: Getting active route count after stop
	count := rm.GetActiveRouteCount()

	// Then: Count should be 0
	if count != 0 {
		t.Errorf("expected 0 active routes after stop, got %d", count)
	}
}

func TestGetActiveRouteCountExcludesFailedRoutes(t *testing.T) {
	// Given: A route manager with one route that will fail (port already bound)
	cfg := &config.Config{
		Routes: []config.Route{
			{Name: "working", Listen: 18231, Upstream: "http://localhost:8031", Server: "server1"},
			{Name: "failing", Listen: 18232, Upstream: "http://localhost:8032", Server: "server2"},
		},
	}
	configMgr := createTestConfigManager(t, cfg)
	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	rm, err := NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	// Block port 18232 to cause the failing route to fail
	blockingServer := &http.Server{
		Addr:              ":18232",
		Handler:           http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		ReadHeaderTimeout: 10 * time.Second,
	}

	blockingDone := make(chan struct{})
	go func() {
		_ = blockingServer.ListenAndServe()
		close(blockingDone)
	}()

	t.Cleanup(func() {
		_ = blockingServer.Close()
		_ = rm.Stop()
		select {
		case <-blockingDone:
		case <-time.After(time.Second):
		}
	})

	// Give blocking server time to start
	time.Sleep(50 * time.Millisecond)

	ctx := context.Background()
	_ = rm.Start(ctx)

	// Give routes time to start/fail
	time.Sleep(200 * time.Millisecond)

	// When: Getting active route count
	count := rm.GetActiveRouteCount()

	// Then: Count should be 1 (only the working route)
	if count != 1 {
		t.Errorf("expected 1 active route (failed route should not count), got %d", count)
	}
}
