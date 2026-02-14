package proxy

import (
	"context"
	"errors"
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

	// ErrConfigMissing is returned when NewRouteManager is called with nil config.
	ErrConfigMissing = errors.New("config cannot be nil")

	// ErrLoggerMissing is returned when NewRouteManager is called with nil logger.
	ErrLoggerMissing = errors.New("logger cannot be nil")

	// ErrMiddlewareMissing is returned when NewRouteManager is called with nil middleware.
	ErrMiddlewareMissing = errors.New("middleware cannot be nil")

	// ErrHealthStoreMissing is returned when NewRouteManager is called with nil health store.
	ErrHealthStoreMissing = errors.New("health store cannot be nil")
)

const (
	defaultShutdownTimeout = 30 * time.Second
)

// RouteManager manages HTTP listeners for all configured routes.
type RouteManager struct {
	config      *config.Config
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

// NewRouteManager creates a new RouteManager instance.
// Returns an error if any parameter is nil.
func NewRouteManager(
	cfg *config.Config,
	log *logger.Logger,
	mw middleware.Middleware,
	store health.HealthStore,
) (*RouteManager, error) {
	if err := validateDependencies(cfg, log, mw, store); err != nil {
		return nil, err
	}

	return &RouteManager{
		config:          cfg,
		logger:          log,
		middleware:      mw,
		healthStore:     store,
		routeMap:        make(map[string]*routeListener),
		shutdownTimeout: defaultShutdownTimeout,
	}, nil
}

func validateDependencies(cfg *config.Config, log *logger.Logger, mw middleware.Middleware, store health.HealthStore) error {
	if cfg == nil {
		return ErrConfigMissing
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
func (m *RouteManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		return ErrAlreadyRunning
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.routes = make([]*routeListener, 0, len(m.config.Routes))

	m.logger.Info("route manager started",
		"total_routes", len(m.config.Routes),
	)

	return nil
}

// Stop gracefully stops all route listeners.
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
	m.mu.Unlock()

	m.logger.Info("route manager stopped successfully")
	return nil
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
