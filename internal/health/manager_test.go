package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

// TestNewHealthManager_ValidParameters tests successful construction.
func TestNewHealthManager_ValidParameters(t *testing.T) {
	// Given: valid parameters
	cfg := &config.Config{
		Servers: make(map[string]config.Server),
	}
	store := newMockHealthStore()
	log := newTestLogger()

	// When: creating a HealthManager
	manager := NewHealthManager(cfg, store, log)

	// Then: should create successfully
	assert.NotNil(t, manager)
	assert.Equal(t, cfg, manager.config)
	assert.Equal(t, store, manager.store)
	assert.NotNil(t, manager.logger)
	assert.Empty(t, manager.workers)
}

// TestNewHealthManager_PanicsOnInvalidParameters tests constructor validation.
func TestNewHealthManager_PanicsOnInvalidParameters(t *testing.T) {
	cfg := &config.Config{}
	store := newMockHealthStore()
	log := newTestLogger()

	testCases := []struct {
		name   string
		config *config.Config
		store  HealthStore
		logger *logger.Logger
	}{
		{
			name:   "nil config",
			config: nil,
			store:  store,
			logger: log,
		},
		{
			name:   "nil store",
			config: cfg,
			store:  nil,
			logger: log,
		},
		{
			name:   "nil logger",
			config: cfg,
			store:  store,
			logger: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Panics(t, func() {
				NewHealthManager(tc.config, tc.store, tc.logger)
			})
		})
	}
}

// TestHealthManager_Start_NoServers tests starting with no servers configured.
func TestHealthManager_Start_NoServers(t *testing.T) {
	// Given: config with no servers
	cfg := &config.Config{
		Servers: make(map[string]config.Server),
	}
	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting the manager
	ctx := context.Background()
	err := manager.Start(ctx)

	// Then: should start successfully with no workers
	assert.NoError(t, err)
	assert.Empty(t, manager.workers)

	// Cleanup
	_ = manager.Stop()
}

// TestHealthManager_Start_SkipsServersWithoutEndpoint tests that servers without endpoints are skipped.
func TestHealthManager_Start_SkipsServersWithoutEndpoint(t *testing.T) {
	// Given: servers with and without health check endpoints
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"with-endpoint": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: "http://example.com/health",
					Interval: 10 * time.Second,
					Timeout:  2 * time.Second,
				},
			},
			"without-endpoint": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: "", // No endpoint
					Interval: 10 * time.Second,
					Timeout:  2 * time.Second,
				},
			},
		},
	}
	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting the manager
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Then: should only create worker for server with endpoint
	assert.Len(t, manager.workers, 1)
	assert.Equal(t, "with-endpoint", manager.workers[0].serverID)

	// Cleanup
	_ = manager.Stop()
}

// TestHealthManager_Start_SkipsServersWithInvalidInterval tests validation of interval.
func TestHealthManager_Start_SkipsServersWithInvalidInterval(t *testing.T) {
	// Given: server with invalid interval
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"invalid-interval": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: "http://example.com/health",
					Interval: 0, // Invalid
					Timeout:  2 * time.Second,
				},
			},
		},
	}
	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting the manager
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Then: should skip the server
	assert.Empty(t, manager.workers)

	// Cleanup
	_ = manager.Stop()
}

// TestHealthManager_Start_AlreadyRunning tests that Start returns error if already running.
func TestHealthManager_Start_AlreadyRunning(t *testing.T) {
	// Given: a running manager
	cfg := &config.Config{
		Servers: make(map[string]config.Server),
	}
	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// When: trying to start again
	err = manager.Start(ctx)

	// Then: should return error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	// Cleanup
	_ = manager.Stop()
}

// TestHealthManager_Stop_NotRunning tests that Stop returns error if not running.
func TestHealthManager_Stop_NotRunning(t *testing.T) {
	// Given: a manager that hasn't been started
	cfg := &config.Config{
		Servers: make(map[string]config.Server),
	}
	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: trying to stop
	err := manager.Stop()

	// Then: should return error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestHealthManager_WorkerPolling tests that workers poll at the configured interval.
func TestHealthManager_WorkerPolling(t *testing.T) {
	// Given: a backend that tracks requests
	var requestCount atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	// Configure server with short interval for testing
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"test-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend.URL,
					Interval: 50 * time.Millisecond, // Short interval for testing
					Timeout:  2 * time.Second,
				},
			},
		},
	}

	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting the manager and waiting for multiple polls
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond) // Wait for ~4 polls (initial + 3 ticks)

	// Then: should have polled multiple times
	assert.GreaterOrEqual(t, int(requestCount.Load()), 3, "should have polled at least 3 times")

	// And: store should be updated with healthy status
	status := store.Get("test-server")
	assert.True(t, status.Healthy)
	assert.False(t, status.LastCheckedAt.IsZero())

	// Cleanup
	err = manager.Stop()
	assert.NoError(t, err)
}

// TestHealthManager_WorkerStopsOnContextCancel tests graceful shutdown.
func TestHealthManager_WorkerStopsOnContextCancel(t *testing.T) {
	// Given: a running manager
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Servers: map[string]config.Server{
			"test-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend.URL,
					Interval: 100 * time.Millisecond,
					Timeout:  2 * time.Second,
				},
			},
		},
	}

	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for at least one check
	time.Sleep(150 * time.Millisecond)

	// When: stopping the manager
	err = manager.Stop()

	// Then: should stop gracefully without error
	assert.NoError(t, err)
	assert.Nil(t, manager.cancel, "cancel should be reset after stop")
}

// TestHealthManager_MultipleWorkers tests managing multiple server workers.
func TestHealthManager_MultipleWorkers(t *testing.T) {
	// Given: multiple backends
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend2.Close()

	cfg := &config.Config{
		Servers: map[string]config.Server{
			"healthy-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend1.URL,
					Interval: 50 * time.Millisecond,
					Timeout:  2 * time.Second,
				},
			},
			"unhealthy-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend2.URL,
					Interval: 50 * time.Millisecond,
					Timeout:  2 * time.Second,
				},
			},
		},
	}

	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting and running
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	time.Sleep(150 * time.Millisecond) // Wait for checks

	// Then: should have 2 workers
	assert.Len(t, manager.workers, 2)

	// And: each should have updated the store
	healthyStatus := store.Get("healthy-server")
	assert.True(t, healthyStatus.Healthy)

	unhealthyStatus := store.Get("unhealthy-server")
	assert.False(t, unhealthyStatus.Healthy)
	assert.NotEmpty(t, unhealthyStatus.LastError)

	// Cleanup
	err = manager.Stop()
	assert.NoError(t, err)
}

// TestHealthManager_StateTransitions tests that state transitions are tracked.
func TestHealthManager_StateTransitions(t *testing.T) {
	// Given: a backend that can change state
	var healthy atomic.Bool
	healthy.Store(true)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer backend.Close()

	cfg := &config.Config{
		Servers: map[string]config.Server{
			"test-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend.URL,
					Interval: 50 * time.Millisecond,
					Timeout:  2 * time.Second,
				},
			},
		},
	}

	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Wait for initial healthy state
	time.Sleep(100 * time.Millisecond)
	status1 := store.Get("test-server")
	assert.True(t, status1.Healthy)

	// When: backend becomes unhealthy
	healthy.Store(false)
	time.Sleep(100 * time.Millisecond)

	// Then: store should reflect unhealthy state
	status2 := store.Get("test-server")
	assert.False(t, status2.Healthy)
	assert.NotEmpty(t, status2.LastError)
	assert.True(t, status2.LastCheckedAt.After(status1.LastCheckedAt))

	// When: backend recovers
	healthy.Store(true)
	time.Sleep(100 * time.Millisecond)

	// Then: store should reflect healthy state again
	status3 := store.Get("test-server")
	assert.True(t, status3.Healthy)
	assert.Empty(t, status3.LastError)

	// Cleanup
	err = manager.Stop()
	assert.NoError(t, err)
}

// TestHealthManager_ImmediateInitialCheck tests that first check happens immediately.
func TestHealthManager_ImmediateInitialCheck(t *testing.T) {
	// Given: a backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Servers: map[string]config.Server{
			"test-server": {
				HealthCheck: config.ServerHealthCheck{
					Endpoint: backend.URL,
					Interval: 10 * time.Second, // Long interval
					Timeout:  2 * time.Second,
				},
			},
		},
	}

	store := newMockHealthStore()
	manager := NewHealthManager(cfg, store, newTestLogger())

	// When: starting the manager
	ctx := context.Background()
	err := manager.Start(ctx)
	require.NoError(t, err)

	// Then: should have performed initial check almost immediately
	time.Sleep(50 * time.Millisecond)
	status := store.Get("test-server")
	assert.False(t, status.LastCheckedAt.IsZero(), "should have performed initial check")

	// Cleanup
	err = manager.Stop()
	assert.NoError(t, err)
}
