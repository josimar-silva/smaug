package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/health"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/store"
)

// --- Fakes ---

// fakeWoLSender records SendWoL calls and returns a configurable error.
type fakeWoLSender struct {
	mu        sync.Mutex
	calls     int
	returnErr error
}

func (f *fakeWoLSender) SendWoL(_ context.Context, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.returnErr
}

func (f *fakeWoLSender) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeHealthStore is a simple in-memory health store for tests.
type fakeHealthStore struct {
	mu     sync.RWMutex
	status map[string]health.ServerHealthStatus
}

const testServerID = "test-server"

func newFakeHealthStore(healthy bool) *fakeHealthStore {
	s := &fakeHealthStore{status: make(map[string]health.ServerHealthStatus)}
	s.status[testServerID] = health.ServerHealthStatus{ServerID: testServerID, Healthy: healthy}
	return s
}

func (s *fakeHealthStore) Get(serverID string) (health.ServerHealthStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	status, exists := s.status[serverID]
	return status, exists
}

func (s *fakeHealthStore) Update(serverID string, status health.ServerHealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[serverID] = status
}

func (s *fakeHealthStore) GetAll() map[string]health.ServerHealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]health.ServerHealthStatus, len(s.status))
	for k, v := range s.status {
		cp[k] = v
	}
	return cp
}

// setHealthy is a test helper to flip the test server's health status in the store.
func (s *fakeHealthStore) setHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status[testServerID] = health.ServerHealthStatus{ServerID: testServerID, Healthy: healthy}
}

// newCoordinatorTestLogger returns a quiet logger for tests.
func newCoordinatorTestLogger() *logger.Logger {
	return logger.New(logger.LevelError, logger.TEXT, nil)
}

// okHandler is a simple downstream handler that records whether it was called.
type okHandler struct {
	called atomic.Bool
}

func (h *okHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.called.Store(true)
	w.WriteHeader(http.StatusOK)
}

// newCoordinator builds a WakeCoordinator with the given config and fakes.
func newCoordinator(
	t *testing.T,
	cfg WakeCoordinatorConfig,
	wol WoLSender,
	hs health.HealthStore,
	downstream http.Handler,
) *WakeCoordinator {
	t.Helper()
	c, err := NewWakeCoordinator(cfg, downstream, wol, hs, newCoordinatorTestLogger(), nil)
	require.NoError(t, err)
	return c
}

// defaultConfig returns a WakeCoordinatorConfig suitable for most tests.
func defaultConfig() WakeCoordinatorConfig {
	return WakeCoordinatorConfig{
		ServerID:     "test-server",
		MachineID:    "saruman",
		WakeTimeout:  2 * time.Second,
		Debounce:     5 * time.Second,
		PollInterval: 10 * time.Millisecond,
	}
}

// --- Construction tests ---

// TestNewWakeCoordinatorSuccess verifies that a valid config produces a coordinator.
func TestNewWakeCoordinatorSuccess(t *testing.T) {
	// Given: valid dependencies
	hs := newFakeHealthStore(true)
	downstream := &okHandler{}
	cfg := defaultConfig()

	// When: creating the coordinator
	c, err := NewWakeCoordinator(cfg, downstream, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then: no error
	assert.NoError(t, err)
	assert.NotNil(t, c)
}

// TestNewWakeCoordinatorRejectsEmptyServerID verifies that an empty server ID is rejected.
func TestNewWakeCoordinatorRejectsEmptyServerID(t *testing.T) {
	// Given: a config with empty server ID
	cfg := defaultConfig()
	cfg.ServerID = ""
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingServerID))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsEmptyMachineID verifies that an empty machine ID is rejected.
func TestNewWakeCoordinatorRejectsEmptyMachineID(t *testing.T) {
	// Given: a config with empty machine ID
	cfg := defaultConfig()
	cfg.MachineID = ""
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingMachineID))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsZeroWakeTimeout verifies that a zero wake timeout is rejected.
func TestNewWakeCoordinatorRejectsZeroWakeTimeout(t *testing.T) {
	// Given: a config with zero wake timeout
	cfg := defaultConfig()
	cfg.WakeTimeout = 0
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidWakeTimeout))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsNilDownstream verifies that a nil downstream is rejected.
func TestNewWakeCoordinatorRejectsNilDownstream(t *testing.T) {
	// Given: nil downstream handler
	cfg := defaultConfig()
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, nil, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingDownstream))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsNilWoLSender verifies that a nil WoL sender is rejected.
func TestNewWakeCoordinatorRejectsNilWoLSender(t *testing.T) {
	// Given: nil WoL sender
	cfg := defaultConfig()
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, nil, hs, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingWoLSender))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsNilHealthStore verifies that a nil health store is rejected.
func TestNewWakeCoordinatorRejectsNilHealthStore(t *testing.T) {
	// Given: nil health store
	cfg := defaultConfig()

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, nil, newCoordinatorTestLogger(), nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingHealthStore))
	assert.Nil(t, c)
}

// TestNewWakeCoordinatorRejectsNilLogger verifies that a nil logger is rejected.
func TestNewWakeCoordinatorRejectsNilLogger(t *testing.T) {
	// Given: nil logger
	cfg := defaultConfig()
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, hs, nil, nil)

	// Then
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingLogger))
	assert.Nil(t, c)
}

// --- Wake success and pass-through tests ---

// TestCoordinatorPassesThroughWhenServerHealthy verifies that a healthy server gets
// requests forwarded immediately without triggering WoL.
func TestCoordinatorPassesThroughWhenServerHealthy(t *testing.T) {
	// Given: server is healthy
	hs := newFakeHealthStore(true)
	downstream := &okHandler{}
	sender := &fakeWoLSender{}
	c := newCoordinator(t, defaultConfig(), sender, hs, downstream)

	// When: request arrives
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c.ServeHTTP(w, r)

	// Then: forwarded to downstream, no WoL sent
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, downstream.called.Load(), "downstream must be called")
	assert.Equal(t, 0, sender.callCount(), "WoL must not be sent when server is healthy")
}

// TestCoordinatorWakesServerAndForwardsRequest verifies the full wake flow:
// server unhealthy → send WoL → store updated healthy → forward request.
func TestCoordinatorWakesServerAndForwardsRequest(t *testing.T) {
	// Given: server is initially unhealthy
	hs := newFakeHealthStore(false)
	downstream := &okHandler{}
	sender := &fakeWoLSender{}

	cfg := defaultConfig()
	cfg.WakeTimeout = 2 * time.Second
	cfg.Debounce = 0 // disable debounce for this test
	cfg.PollInterval = 10 * time.Millisecond
	c := newCoordinator(t, cfg, sender, hs, downstream)

	// Simulate HealthManager updating the store to healthy after a short delay
	go func() {
		time.Sleep(30 * time.Millisecond)
		hs.setHealthy(true)
	}()

	// When: request arrives for unhealthy server
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c.ServeHTTP(w, r)

	// Then: WoL sent once, request forwarded once store reports healthy
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, downstream.called.Load(), "downstream must be called after wake")
	assert.Equal(t, 1, sender.callCount(), "WoL must be sent exactly once")
}

// TestCoordinatorReturns503OnWakeTimeout verifies that a 503 is returned when the
// server does not become healthy within the wake timeout.
func TestCoordinatorReturns503OnWakeTimeout(t *testing.T) {
	// Given: server stays unhealthy forever (store never updated)
	hs := newFakeHealthStore(false)
	downstream := &okHandler{}
	sender := &fakeWoLSender{}

	cfg := defaultConfig()
	cfg.WakeTimeout = 100 * time.Millisecond // short timeout so test is fast
	cfg.Debounce = 0
	cfg.PollInterval = 10 * time.Millisecond
	c := newCoordinator(t, cfg, sender, hs, downstream)

	// When: request arrives for unhealthy server
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	c.ServeHTTP(w, r)

	// Then: 503 returned, downstream never called
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.False(t, downstream.called.Load(), "downstream must NOT be called on timeout")
	assert.Equal(t, 1, sender.callCount(), "WoL must be sent exactly once")
}

// --- Debounce tests ---

// TestCoordinatorDebouncesWoLCalls verifies that a second WoL is not sent within the
// debounce window, even if the server is unhealthy.
func TestCoordinatorDebouncesWoLCalls(t *testing.T) {
	// Given: server is unhealthy; first request wakes it (WoL sent).
	// Then server becomes healthy. Then server goes unhealthy again within debounce window.
	hs := newFakeHealthStore(false)
	downstream := &okHandler{}
	sender := &fakeWoLSender{}

	cfg := defaultConfig()
	cfg.WakeTimeout = 2 * time.Second
	cfg.Debounce = 5 * time.Second // long debounce
	cfg.PollInterval = 10 * time.Millisecond
	c := newCoordinator(t, cfg, sender, hs, downstream)

	// First request: triggers WoL; simulate server becoming healthy after WoL
	go func() {
		time.Sleep(30 * time.Millisecond)
		hs.setHealthy(true)
	}()
	w1 := httptest.NewRecorder()
	c.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, 1, sender.callCount(), "first request must trigger WoL")

	// Now make the store report unhealthy again (simulating server going down again briefly)
	hs.setHealthy(false)

	// Second request within debounce window: must NOT trigger another WoL
	// Simulate server becoming healthy again (via background HealthManager)
	go func() {
		time.Sleep(30 * time.Millisecond)
		hs.setHealthy(true)
	}()
	w2 := httptest.NewRecorder()
	c.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/", nil))

	// Then: WoL count unchanged (still 1), debounce suppressed the second trigger
	assert.Equal(t, 1, sender.callCount(), "second WoL must be suppressed within debounce window")
}

// --- Concurrency tests ---

// TestCoordinatorSingleWoLForConcurrentRequests verifies that exactly one WoL is sent
// when 100 concurrent requests arrive for an unhealthy server.
func TestCoordinatorSingleWoLForConcurrentRequests(t *testing.T) {
	// Given: server is unhealthy; WoL takes a little time so all 100 requests arrive
	// before it completes.
	hs := newFakeHealthStore(false)

	var wolWg sync.WaitGroup
	wolWg.Add(1)
	slowSender := &blockingWoLSender{ready: &wolWg}

	cfg := defaultConfig()
	cfg.WakeTimeout = 5 * time.Second
	cfg.Debounce = 0
	cfg.PollInterval = 10 * time.Millisecond
	c := newCoordinator(t, cfg, slowSender, hs, &okHandler{})

	const concurrency = 100

	var wg sync.WaitGroup
	wg.Add(concurrency)

	// Unblock the WoL sender slightly after all goroutines are running,
	// then simulate the server becoming healthy.
	go func() {
		time.Sleep(20 * time.Millisecond)
		wolWg.Done() // unblock the WoL sender
		time.Sleep(30 * time.Millisecond)
		hs.setHealthy(true)
	}()

	// When: 100 requests arrive concurrently
	for range concurrency {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			c.ServeHTTP(w, r)
		}()
	}

	wg.Wait()

	// Then: WoL sent exactly once regardless of 100 concurrent requests
	assert.Equal(t, 1, slowSender.callCount(), "exactly one WoL must be sent for concurrent requests")
}

// blockingWoLSender blocks until ready is Done, then records the call.
type blockingWoLSender struct {
	mu    sync.Mutex
	calls int
	ready *sync.WaitGroup
}

func (s *blockingWoLSender) SendWoL(_ context.Context, _ string) error {
	s.ready.Wait() // block until unblocked
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return nil
}

func (s *blockingWoLSender) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// --- PollInterval default ---

// TestNewWakeCoordinatorDefaultPollInterval verifies that a zero PollInterval is replaced
// with the package default.
func TestNewWakeCoordinatorDefaultPollInterval(t *testing.T) {
	// Given: config with PollInterval = 0
	cfg := defaultConfig()
	cfg.PollInterval = 0
	hs := newFakeHealthStore(true)

	// When
	c, err := NewWakeCoordinator(cfg, &okHandler{}, &fakeWoLSender{}, hs, newCoordinatorTestLogger(), nil)

	// Then: coordinator uses the package default
	require.NoError(t, err)
	assert.Equal(t, defaultPollInterval, c.pollInterval)
}

// TestCoordinatorWoLSenderFailureStillPolls verifies that a WoL send error does not
// abort the poll loop — the server might already be waking up from a previous trigger.
func TestCoordinatorWoLSenderFailureStillPolls(t *testing.T) {
	// Given: WoL sender fails, but the server becomes healthy via the store anyway
	hs := newFakeHealthStore(false)
	downstream := &okHandler{}
	sender := &fakeWoLSender{returnErr: errors.New("gwaihir unavailable")}

	cfg := defaultConfig()
	cfg.WakeTimeout = 2 * time.Second
	cfg.Debounce = 0
	cfg.PollInterval = 10 * time.Millisecond
	c := newCoordinator(t, cfg, sender, hs, downstream)

	// Simulate HealthManager marking the server healthy even though our WoL failed
	go func() {
		time.Sleep(30 * time.Millisecond)
		hs.setHealthy(true)
	}()

	// When: request arrives
	w := httptest.NewRecorder()
	c.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	// Then: request still forwarded (server became healthy via store even though WoL failed)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, downstream.called.Load())
}

// TestCoordinatorContextCancelledDuringWake verifies that if the request context is
// cancelled while waiting for the server to wake, the coordinator returns 503.
func TestCoordinatorContextCancelledDuringWake(t *testing.T) {
	// Given: server stays unhealthy forever
	hs := newFakeHealthStore(false)
	downstream := &okHandler{}
	sender := &fakeWoLSender{}

	cfg := defaultConfig()
	cfg.WakeTimeout = 10 * time.Second // long enough that context cancellation fires first
	cfg.Debounce = 0
	cfg.PollInterval = 50 * time.Millisecond
	c := newCoordinator(t, cfg, sender, hs, downstream)

	// When: request with a short-lived context
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
	c.ServeHTTP(w, r)

	// Then: 503, no downstream call
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.False(t, downstream.called.Load())
}

// --- SetWakeOptions and wrapWithWakeCoordinator tests ---

// createWakeTestConfig builds a ConfigManager backed by a YAML file so RouteManager
// can be used in tests that exercise wrapWithWakeCoordinator.
func createWakeTestConfig(t *testing.T, cfg *config.Config) *config.ConfigManager {
	t.Helper()
	if cfg.Servers == nil {
		cfg.Servers = make(map[string]config.Server)
	}
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	data, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0600))
	log := logger.New(logger.LevelError, logger.TEXT, nil)
	mgr, err := config.NewManager(configPath, log)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Stop() })
	return mgr
}

// buildTestRouteManager creates a RouteManager suitable for wrapWithWakeCoordinator tests.
func buildTestRouteManager(t *testing.T, cfg *config.Config) *RouteManager {
	t.Helper()
	configMgr := createWakeTestConfig(t, cfg)
	healthStore := store.NewInMemoryHealthStore()
	log := logger.New(logger.LevelError, logger.TEXT, nil)
	rm, err := NewRouteManager(configMgr, log, middleware.Chain(), healthStore)
	require.NoError(t, err)
	return rm
}

// TestSetWakeOptionsStoresOptions verifies that SetWakeOptions populates the field.
func TestSetWakeOptionsStoresOptions(t *testing.T) {
	// Given: a RouteManager
	rm := buildTestRouteManager(t, &config.Config{})

	// When: SetWakeOptions is called
	opts := WakeOptions{Sender: &fakeWoLSender{}}
	rm.SetWakeOptions(opts)

	// Then: the field is populated
	assert.NotNil(t, rm.wakeOptions)
}

// TestWrapWithWakeCoordinatorNoWakeOptions verifies that when WakeOptions are not set
// the original handler is returned unchanged.
func TestWrapWithWakeCoordinatorNoWakeOptions(t *testing.T) {
	// Given: a RouteManager without WakeOptions
	rm := buildTestRouteManager(t, &config.Config{})
	original := &okHandler{}
	route := config.Route{Name: "test", Server: "server1"}

	// When: wrapping the handler
	wrapped, _ := rm.wrapWithWakeCoordinator(route, original)

	// Then: original handler returned
	assert.Equal(t, original, wrapped)
}

// TestWrapWithWakeCoordinatorEmptyServerName verifies that a route with no server name
// skips wake coordination.
func TestWrapWithWakeCoordinatorEmptyServerName(t *testing.T) {
	// Given: a RouteManager with WakeOptions but a route with no server name
	rm := buildTestRouteManager(t, &config.Config{})
	rm.SetWakeOptions(WakeOptions{Sender: &fakeWoLSender{}})
	original := &okHandler{}

	// When
	wrapped, _ := rm.wrapWithWakeCoordinator(config.Route{Name: "test", Server: ""}, original)

	// Then: original handler returned
	assert.Equal(t, original, wrapped)
}

// TestWrapWithWakeCoordinatorWoLDisabled verifies that when WoL is disabled in config
// the original handler is returned.
func TestWrapWithWakeCoordinatorWoLDisabled(t *testing.T) {
	// Given: server exists but WoL is disabled
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {WakeOnLan: config.WakeOnLanConfig{Enabled: false}},
		},
	}
	rm := buildTestRouteManager(t, cfg)
	rm.SetWakeOptions(WakeOptions{Sender: &fakeWoLSender{}})
	original := &okHandler{}

	// When
	wrapped, _ := rm.wrapWithWakeCoordinator(config.Route{Name: "test", Server: "server1"}, original)

	// Then: original handler returned
	assert.Equal(t, original, wrapped)
}

// TestWrapWithWakeCoordinatorSuccess verifies that when all conditions are met a
// WakeCoordinator is returned (not the original handler), along with a non-nil closer.
func TestWrapWithWakeCoordinatorSuccess(t *testing.T) {
	// Given: server with WoL fully configured
	cfg := &config.Config{
		Servers: map[string]config.Server{
			"server1": {WakeOnLan: config.WakeOnLanConfig{
				Enabled:   true,
				MachineID: "saruman",
				Timeout:   5 * time.Second,
				Debounce:  5 * time.Second,
			}},
		},
	}
	rm := buildTestRouteManager(t, cfg)
	rm.SetWakeOptions(WakeOptions{
		Sender: &fakeWoLSender{},
	})
	original := &okHandler{}

	// When
	wrapped, closer := rm.wrapWithWakeCoordinator(config.Route{Name: "test", Server: "server1"}, original)

	// Then: a WakeCoordinator is returned (not the original handler), with a closer
	_, ok := wrapped.(*WakeCoordinator)
	assert.True(t, ok, "expected a *WakeCoordinator to be returned")
	assert.NotNil(t, closer, "expected a non-nil closer")
	t.Cleanup(func() { _ = closer.Close() })
}
