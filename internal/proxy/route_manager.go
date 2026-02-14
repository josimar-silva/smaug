package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/health"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
)

var (
	// ErrAlreadyRunning is returned when Start is called on an already running RouteManager.
	ErrAlreadyRunning = errors.New("route manager is already running")

	// ErrNotRunning is returned when Stop is called on a non-running RouteManager.
	ErrNotRunning = errors.New("route manager is not running")

	// ErrShutdownTimeout is returned when graceful shutdown exceeds the timeout.
	ErrShutdownTimeout = errors.New("shutdown timeout: some listeners did not stop in time")

	// ErrConfigManagerMissing is returned when NewRouteManager is called with nil config manager.
	ErrConfigManagerMissing = errors.New("config manager cannot be nil")

	// ErrLoggerMissing is returned when NewRouteManager is called with nil logger.
	ErrLoggerMissing = errors.New("logger cannot be nil")

	// ErrMiddlewareMissing is returned when NewRouteManager is called with nil middleware.
	ErrMiddlewareMissing = errors.New("middleware cannot be nil")

	// ErrHealthStoreMissing is returned when NewRouteManager is called with nil health store.
	ErrHealthStoreMissing = errors.New("health store cannot be nil")

	// ErrReloadFailed is returned when config reload fails.
	ErrReloadFailed = errors.New("config reload failed")

	// ErrRouteStopFailed is returned when stopping a route fails during reload.
	ErrRouteStopFailed = errors.New("failed to stop route")

	// ErrRouteStartFailed is returned when starting a route fails during reload.
	ErrRouteStartFailed = errors.New("failed to start route")
)

const (
	defaultShutdownTimeout = 30 * time.Second
)

// RouteManager manages HTTP listeners for all configured routes.
type RouteManager struct {
	configMgr   *config.ConfigManager
	logger      *logger.Logger
	middleware  middleware.Middleware
	healthStore health.HealthStore

	routes          []*routeListener
	routeMap        map[string]*routeListener
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex
	shutdownTimeout time.Duration

	reloadWg sync.WaitGroup
	reloadMu sync.Mutex
}

// routeListener represents a single HTTP listener for a route.
type routeListener struct {
	route    config.Route
	server   *http.Server
	handler  http.Handler
	logger   *logger.Logger
	state    listenerState
	stateMu  sync.RWMutex
	startErr error
	started  chan struct{} // Closed when listener reaches running or failed state
}

// listenerState represents the lifecycle state of a listener.
type listenerState int

const (
	stateInitial listenerState = iota
	stateStarting
	stateRunning
	stateStopping
	stateStopped
	stateFailed
)

// String returns human-readable state name for logging.
func (s listenerState) String() string {
	switch s {
	case stateInitial:
		return "initial"
	case stateStarting:
		return "starting"
	case stateRunning:
		return "running"
	case stateStopping:
		return "stopping"
	case stateStopped:
		return "stopped"
	case stateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// RouteInfo contains runtime information about a route's listener.
type RouteInfo struct {
	Name     string
	Port     int
	Upstream string
	Server   string
	State    string
}

// NewRouteManager creates a new RouteManager instance with hot-reload support.
//
// Parameters:
//   - configMgr: Configuration manager with hot-reload capability
//   - log: Logger for structured logging
//   - mw: Middleware chain to apply to all routes
//   - store: Health status store for backend servers
//
// Returns a new RouteManager instance or an error if any parameter is nil.
func NewRouteManager(
	configMgr *config.ConfigManager,
	log *logger.Logger,
	mw middleware.Middleware,
	store health.HealthStore,
) (*RouteManager, error) {
	if err := validateDependencies(configMgr, log, mw, store); err != nil {
		return nil, err
	}

	return &RouteManager{
		configMgr:       configMgr,
		logger:          log,
		middleware:      mw,
		healthStore:     store,
		routeMap:        make(map[string]*routeListener),
		shutdownTimeout: defaultShutdownTimeout,
	}, nil
}

func validateDependencies(configMgr *config.ConfigManager, log *logger.Logger, mw middleware.Middleware, store health.HealthStore) error {
	if configMgr == nil {
		return ErrConfigManagerMissing
	}
	if log == nil {
		return ErrLoggerMissing
	}
	if mw == nil {
		return ErrMiddlewareMissing
	}
	if store == nil {
		return ErrHealthStoreMissing
	}

	return nil
}

// Start initializes and starts HTTP listeners for all configured routes.
// It also spawns a background goroutine to watch for config reloads.
func (m *RouteManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return ErrAlreadyRunning
	}

	cfg := m.configMgr.GetConfig()
	if cfg == nil {
		return fmt.Errorf("%w: config manager returned nil config", ErrReloadFailed)
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.routes = make([]*routeListener, 0, len(cfg.Routes))

	for _, route := range cfg.Routes {
		listener := m.createListener(route)
		key := routeKey(route)
		m.routes = append(m.routes, listener)
		m.routeMap[key] = listener

		m.wg.Add(1)
		go m.runListener(listener)
	}

	m.logger.Info("route manager started",
		"total_routes", len(cfg.Routes),
	)

	go m.watchConfigReloads()

	return nil
}

// Stop gracefully stops all route listeners and the config reload watcher.
func (m *RouteManager) Stop() error {
	m.mu.Lock()
	if m.cancel == nil {
		m.mu.Unlock()
		return ErrNotRunning
	}

	m.logger.Info("stopping route manager",
		"route_count", len(m.routes),
		"shutdown_timeout", m.shutdownTimeout,
	)

	m.cancel()
	m.cancel = nil

	routes := m.routes
	m.mu.Unlock()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), m.shutdownTimeout)
	defer shutdownCancel()

	m.shutdownRoutes(shutdownCtx, routes)

	m.reloadWg.Wait()

	return m.waitForShutdown(shutdownCtx)
}

func (m *RouteManager) shutdownRoutes(ctx context.Context, routes []*routeListener) {
	for _, listener := range routes {
		listener.stateMu.Lock()
		listener.state = stateStopping
		listener.stateMu.Unlock()

		if err := listener.server.Shutdown(ctx); err != nil {
			m.logger.Warn("listener shutdown error",
				"route", listener.route.Name,
				"error", err,
			)
			_ = listener.server.Close()
		}

		listener.stateMu.Lock()
		listener.state = stateStopped
		listener.stateMu.Unlock()
	}
}

func (m *RouteManager) waitForShutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("route manager stopped successfully")
		return nil
	case <-ctx.Done():
		m.logger.Error("route manager shutdown timeout",
			"timeout", m.shutdownTimeout,
		)
		return ErrShutdownTimeout
	}
}

// GetActiveRoutes returns information about all active route listeners.
func (m *RouteManager) GetActiveRoutes() []RouteInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.routes) == 0 {
		return []RouteInfo{}
	}

	info := make([]RouteInfo, 0, len(m.routes))
	for _, listener := range m.routes {
		listener.stateMu.RLock()
		state := listener.state.String()
		listener.stateMu.RUnlock()

		info = append(info, RouteInfo{
			Name:     listener.route.Name,
			Port:     listener.route.Listen,
			Upstream: listener.route.Upstream,
			Server:   listener.route.Server,
			State:    state,
		})
	}

	return info
}

func (m *RouteManager) createListener(route config.Route) *routeListener {
	proxyHandler := NewProxyHandler(route.Upstream, m.logger)
	wrappedHandler := m.middleware(proxyHandler)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", route.Listen),
		Handler:           wrappedHandler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	return &routeListener{
		route:   route,
		server:  server,
		handler: wrappedHandler,
		logger:  m.logger,
		state:   stateInitial,
		started: make(chan struct{}),
	}
}

func (m *RouteManager) runListener(listener *routeListener) {
	defer m.wg.Done()

	listener.stateMu.Lock()
	listener.state = stateStarting
	listener.stateMu.Unlock()

	m.logger.Info("starting route listener",
		"route", listener.route.Name,
		"port", listener.route.Listen,
		"upstream", listener.route.Upstream,
		"server", listener.route.Server,
	)

	// Signal that we're about to attempt ListenAndServe
	// We'll close this channel once it succeeds or fails
	go func() {
		time.Sleep(100 * time.Millisecond)
		listener.stateMu.Lock()
		state := listener.state
		listener.stateMu.Unlock()
		// If still in starting state after 100ms, assume listening succeeded
		if state == stateStarting {
			listener.stateMu.Lock()
			listener.state = stateRunning
			listener.stateMu.Unlock()
			close(listener.started)
		}
	}()

	err := listener.server.ListenAndServe()

	if err != nil && err != http.ErrServerClosed {
		listener.stateMu.Lock()
		listener.state = stateFailed
		listener.startErr = err
		listener.stateMu.Unlock()

		m.logger.Error("route listener failed",
			"route", listener.route.Name,
			"port", listener.route.Listen,
			"error", err,
		)
		close(listener.started)
		return
	}

	m.logger.Debug("route listener stopped",
		"route", listener.route.Name,
		"port", listener.route.Listen,
	)
}

func (m *RouteManager) stopRoute(key string) error {
	m.mu.RLock()
	listener, exists := m.routeMap[key]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("route %s not found", key)
	}

	m.logger.Info("stopping route",
		"route", listener.route.Name,
		"port", listener.route.Listen,
	)

	listener.stateMu.Lock()
	listener.state = stateStopping
	listener.stateMu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), m.shutdownTimeout)
	defer cancel()

	if err := listener.server.Shutdown(shutdownCtx); err != nil {
		_ = listener.server.Close()
		return fmt.Errorf("shutdown failed: %w", err)
	}

	listener.stateMu.Lock()
	listener.state = stateStopped
	listener.stateMu.Unlock()

	m.logger.Info("route stopped successfully",
		"route", listener.route.Name,
		"port", listener.route.Listen,
	)

	return nil
}

func (m *RouteManager) startRoute(route config.Route) error {
	listener := m.createListener(route)

	m.mu.Lock()
	key := routeKey(route)
	m.routeMap[key] = listener
	m.routes = append(m.routes, listener)
	m.mu.Unlock()

	m.wg.Add(1)
	go m.runListener(listener)

	// Wait for listener to reach running or failed state (with timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	select {
	case <-listener.started:
	case <-ctx.Done():
		return fmt.Errorf("route startup timeout: %w", ctx.Err())
	}

	listener.stateMu.RLock()
	state := listener.state
	err := listener.startErr
	listener.stateMu.RUnlock()

	if state == stateFailed {
		return fmt.Errorf("route failed to start: %w", err)
	}

	m.logger.Info("route started successfully",
		"route", route.Name,
		"port", route.Listen,
	)

	return nil
}

func (m *RouteManager) cleanupStoppedRoutes(keys []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keysMap := make(map[string]bool)
	for _, key := range keys {
		keysMap[key] = true
		delete(m.routeMap, key)
	}

	activeRoutes := make([]*routeListener, 0, len(m.routes))
	for _, listener := range m.routes {
		key := routeKey(listener.route)
		if !keysMap[key] {
			activeRoutes = append(activeRoutes, listener)
		}
	}
	m.routes = activeRoutes
}

func (m *RouteManager) applyRouteChanges(changes RouteChanges) error {
	var errors []error

	routesToStop := make([]string, 0, len(changes.Removed)+len(changes.Modified))
	routesToStop = append(routesToStop, changes.Removed...)

	for _, route := range changes.Modified {
		routesToStop = append(routesToStop, routeKey(route))
	}

	m.logger.Info("stopping routes", "count", len(routesToStop))
	for _, key := range routesToStop {
		if err := m.stopRoute(key); err != nil {
			m.logger.Error("failed to stop route",
				"route_key", key,
				"error", err,
			)
			errors = append(errors, fmt.Errorf("%w: %s: %v", ErrRouteStopFailed, key, err))
		}
	}

	m.cleanupStoppedRoutes(routesToStop)

	routesToStart := make([]config.Route, 0, len(changes.Added)+len(changes.Modified))
	routesToStart = append(routesToStart, changes.Added...)
	routesToStart = append(routesToStart, changes.Modified...)

	m.logger.Info("starting routes", "count", len(routesToStart))
	for _, route := range routesToStart {
		if err := m.startRoute(route); err != nil {
			m.logger.Error("failed to start route",
				"route", route.Name,
				"port", route.Listen,
				"error", err,
			)
			errors = append(errors, fmt.Errorf("%w: %s:%d: %v", ErrRouteStartFailed, route.Name, route.Listen, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%w: reload completed with %d errors: %v", ErrReloadFailed, len(errors), errors)
	}

	return nil
}

// watchConfigReloads monitors the ConfigManager for reload signals and
// orchestrates graceful route transitions.
func (m *RouteManager) watchConfigReloads() {
	m.reloadWg.Add(1)
	defer m.reloadWg.Done()

	reloadChan := m.configMgr.ReloadSignal()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Debug("stopping config reload watcher")
			return

		case <-reloadChan:
			m.logger.Info("config reload signal received")
			if err := m.handleReload(); err != nil {
				m.logger.Error("config reload failed", "error", err)
			}
		}
	}
}

// handleReload processes a configuration reload by computing diffs
// and applying changes to running routes.
func (m *RouteManager) handleReload() error {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	m.logger.Info("starting route reload")

	newConfig := m.configMgr.GetConfig()
	if newConfig == nil {
		return fmt.Errorf("%w: config manager returned nil config", ErrReloadFailed)
	}

	m.mu.RLock()
	oldRoutes := make([]config.Route, 0, len(m.routes))
	for _, listener := range m.routes {
		oldRoutes = append(oldRoutes, listener.route)
	}
	m.mu.RUnlock()

	changes := detectChanges(oldRoutes, newConfig.Routes)

	m.logger.Info("route changes detected",
		"added", len(changes.Added),
		"removed", len(changes.Removed),
		"modified", len(changes.Modified),
		"unchanged", len(changes.Unchanged),
	)

	if len(changes.Added) == 0 && len(changes.Removed) == 0 && len(changes.Modified) == 0 {
		m.logger.Info("no route changes detected, skipping reload")
		return nil
	}

	if err := m.applyRouteChanges(changes); err != nil {
		return err
	}

	m.logger.Info("route reload completed successfully")
	return nil
}
