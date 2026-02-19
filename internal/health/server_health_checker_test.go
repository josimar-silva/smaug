package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

const (
	healthCheckFailuresMetricName = "smaug_health_check_failures_total"
)

type mockHealthStore struct {
	mu       sync.RWMutex
	statuses map[string]ServerHealthStatus
}

func newMockHealthStore() *mockHealthStore {
	return &mockHealthStore{
		statuses: make(map[string]ServerHealthStatus),
	}
}

func (m *mockHealthStore) Get(serverID string) ServerHealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status, exists := m.statuses[serverID]
	if !exists {
		return ServerHealthStatus{
			ServerID: serverID,
			Healthy:  false,
		}
	}
	return status
}

func (m *mockHealthStore) Update(serverID string, status ServerHealthStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.statuses[serverID] = status
}

func (m *mockHealthStore) GetAll() map[string]ServerHealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]ServerHealthStatus, len(m.statuses))
	for k, v := range m.statuses {
		result[k] = v
	}
	return result
}

// TestNewServerHealthCheckerValidParameters tests successful construction.
func TestNewServerHealthCheckerValidParameters(t *testing.T) {
	// Given: valid parameters
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()

	// When: creating a ServerHealthChecker
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// Then: should create successfully
	assert.NotNil(t, serverChecker)
	assert.Equal(t, "saruman", serverChecker.serverID)
	assert.NotNil(t, serverChecker.checker)
	assert.NotNil(t, serverChecker.store)
	assert.NotNil(t, serverChecker.logger)
}

// TestNewServerHealthCheckerPanicsOnInvalidParameters tests constructor validation.
func TestNewServerHealthCheckerPanicsOnInvalidParameters(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	log := newTestLogger()

	testCases := []struct {
		name      string
		serverID  string
		checker   *HealthChecker
		store     HealthStore
		logger    *logger.Logger
		wantPanic bool
	}{
		{
			name:      "empty serverID",
			serverID:  "",
			checker:   checker,
			store:     store,
			logger:    log,
			wantPanic: true,
		},
		{
			name:      "nil checker",
			serverID:  "saruman",
			checker:   nil,
			store:     store,
			logger:    log,
			wantPanic: true,
		},
		{
			name:      "nil store",
			serverID:  "saruman",
			checker:   checker,
			store:     nil,
			logger:    log,
			wantPanic: true,
		},
		{
			name:      "nil logger",
			serverID:  "saruman",
			checker:   checker,
			store:     store,
			logger:    nil,
			wantPanic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantPanic {
				assert.Panics(t, func() {
					NewServerHealthChecker(tc.serverID, tc.checker, tc.store, tc.logger, nil)
				})
			} else {
				assert.NotPanics(t, func() {
					NewServerHealthChecker(tc.serverID, tc.checker, tc.store, tc.logger, nil)
				})
			}
		})
	}
}

// TestServerHealthCheckerCheckSuccess tests successful health check.
func TestServerHealthCheckerCheckSuccess(t *testing.T) {
	// Given: a healthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// When: performing a health check
	ctx := context.Background()
	status, err := serverChecker.Check(ctx)

	// Then: should succeed
	assert.NoError(t, err)
	assert.Equal(t, "saruman", status.ServerID)
	assert.True(t, status.Healthy)
	assert.Empty(t, status.LastError)
	assert.False(t, status.LastCheckedAt.IsZero())

	// And: the store should be updated
	storedStatus := store.Get("saruman")
	assert.True(t, storedStatus.Healthy)
	assert.Equal(t, status.LastCheckedAt, storedStatus.LastCheckedAt)
}

// TestServerHealthCheckerCheckFailure tests failed health check.
func TestServerHealthCheckerCheckFailure(t *testing.T) {
	// Given: an unhealthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// When: performing a health check
	ctx := context.Background()
	status, err := serverChecker.Check(ctx)

	// Then: should fail
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHealthCheckFailed))
	assert.Equal(t, "saruman", status.ServerID)
	assert.False(t, status.Healthy)
	assert.NotEmpty(t, status.LastError)
	assert.Contains(t, status.LastError, "500")

	// And: the store should be updated with unhealthy status
	storedStatus := store.Get("saruman")
	assert.False(t, storedStatus.Healthy)
	assert.NotEmpty(t, storedStatus.LastError)
}

// TestServerHealthCheckerCheckNetworkError tests network failure handling.
func TestServerHealthCheckerCheckNetworkError(t *testing.T) {
	// Given: an unreachable endpoint
	checker := NewHealthChecker("http://192.0.2.1:9999/health", 100*time.Millisecond, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// When: performing a health check
	ctx := context.Background()
	status, err := serverChecker.Check(ctx)

	// Then: should fail with network error
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrHealthCheckNetworkError))
	assert.Equal(t, "saruman", status.ServerID)
	assert.False(t, status.Healthy)
	assert.NotEmpty(t, status.LastError)

	// And: the store should be updated
	storedStatus := store.Get("saruman")
	assert.False(t, storedStatus.Healthy)
}

// TestServerHealthCheckerCheckContextCancellation tests cancellation handling.
func TestServerHealthCheckerCheckContextCancellation(t *testing.T) {
	// Given: a slow backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// When: performing a health check with a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	status, err := serverChecker.Check(ctx)

	// Then: should fail
	assert.Error(t, err)
	assert.False(t, status.Healthy)

	// And: the store should be updated
	storedStatus := store.Get("saruman")
	assert.False(t, storedStatus.Healthy)
}

// TestServerHealthCheckerCheckUpdatesTimestamp tests that timestamps are updated.
func TestServerHealthCheckerCheckUpdatesTimestamp(t *testing.T) {
	// Given: a healthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	ctx := context.Background()

	// When: performing first check
	firstStatus, _ := serverChecker.Check(ctx)
	time.Sleep(10 * time.Millisecond)

	// And: performing second check
	secondStatus, _ := serverChecker.Check(ctx)

	// Then: timestamps should be different
	assert.True(t, secondStatus.LastCheckedAt.After(firstStatus.LastCheckedAt),
		"second check timestamp should be after first check timestamp")
}

// TestServerHealthCheckerCheckTransitionFromHealthyToUnhealthy tests state transitions.
func TestServerHealthCheckerCheckTransitionFromHealthyToUnhealthy(t *testing.T) {
	// Given: a backend that can change state
	healthy := true
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	ctx := context.Background()

	// When: first check with healthy backend
	status1, err1 := serverChecker.Check(ctx)
	assert.NoError(t, err1)
	assert.True(t, status1.Healthy)
	assert.Empty(t, status1.LastError)

	// And: second check with unhealthy backend
	healthy = false
	status2, err2 := serverChecker.Check(ctx)
	assert.Error(t, err2)
	assert.False(t, status2.Healthy)
	assert.NotEmpty(t, status2.LastError)

	// Then: store should reflect the unhealthy state
	storedStatus := store.Get("saruman")
	assert.False(t, storedStatus.Healthy)
	assert.NotEmpty(t, storedStatus.LastError)
}

// TestServerHealthCheckerCheckRecordsMetricsOnSuccess tests that metrics are recorded on successful check.
func TestServerHealthCheckerCheckRecordsMetricsOnSuccess(t *testing.T) {
	// Given: a healthy backend and metrics registry
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	// When: performing a health check
	ctx := context.Background()
	status, checkErr := serverChecker.Check(ctx)

	// Then: should succeed
	assert.NoError(t, checkErr)
	assert.True(t, status.Healthy)

	// And: health check failure metrics should not be recorded for success
	// We verify this by checking that no failures were recorded
	families, err := m.Gatherer().Gather()
	assert.NoError(t, err)

	healthCheckFailuresFound := false
	for _, family := range families {
		if family.GetName() == healthCheckFailuresMetricName {
			healthCheckFailuresFound = true
			// Should have 0 samples for successful check
			assert.Empty(t, family.GetMetric(), "no failures should be recorded on successful health check")
		}
	}

	// It's ok if the metric isn't found yet (first check), but if it exists, it should be empty
	_ = healthCheckFailuresFound
}

// TestServerHealthCheckerCheckRecordsMetricsOnUnhealthyStatus tests that metrics are recorded on unhealthy status.
func TestServerHealthCheckerCheckRecordsMetricsOnUnhealthyStatus(t *testing.T) {
	// Given: an unhealthy backend and metrics registry
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	// When: performing a health check
	ctx := context.Background()
	status, checkErr := serverChecker.Check(ctx)

	// Then: should fail
	assert.Error(t, checkErr)
	assert.False(t, status.Healthy)

	// And: health check failure metrics should be recorded with "unhealthy_status" reason
	families, err := m.Gatherer().Gather()
	assert.NoError(t, err)

	found := false
	for _, family := range families {
		if family.GetName() == healthCheckFailuresMetricName {
			found = true
			assert.NotEmpty(t, family.GetMetric(), "health check failure metrics should be recorded")
			// Verify the failure reason is "unhealthy_status"
			for _, metric := range family.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "reason" {
						assert.Equal(t, "unhealthy_status", label.GetValue())
					}
				}
			}
		}
	}
	assert.True(t, found, healthCheckFailuresMetricName+" metric should exist")
}

// TestServerHealthCheckerCheckRecordsMetricsOnNetworkError tests that metrics are recorded on network error.
func TestServerHealthCheckerCheckRecordsMetricsOnNetworkError(t *testing.T) {
	// Given: an unreachable endpoint and metrics registry
	checker := NewHealthChecker("http://192.0.2.1:9999/health", 100*time.Millisecond, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	// When: performing a health check
	ctx := context.Background()
	status, checkErr := serverChecker.Check(ctx)

	// Then: should fail with network error
	assert.Error(t, checkErr)
	assert.False(t, status.Healthy)

	// And: health check failure metrics should be recorded with "network_error" reason
	families, err := m.Gatherer().Gather()
	assert.NoError(t, err)

	found := false
	for _, family := range families {
		if family.GetName() == healthCheckFailuresMetricName {
			found = true
			assert.NotEmpty(t, family.GetMetric(), "health check failure metrics should be recorded")
			// Verify the failure reason is "network_error"
			for _, metric := range family.GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "reason" {
						assert.Equal(t, "network_error", label.GetValue())
					}
				}
			}
		}
	}
	assert.True(t, found, healthCheckFailuresMetricName+" metric should exist")
}

// TestServerHealthCheckerCheckRecordsMetricsOnTimeout tests that metrics are recorded on timeout.
func TestServerHealthCheckerCheckRecordsMetricsOnTimeout(t *testing.T) {
	// Given: a slow backend and metrics registry
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 50*time.Millisecond, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	// When: performing a health check that times out
	ctx := context.Background()
	status, checkErr := serverChecker.Check(ctx)

	// Then: should fail
	assert.Error(t, checkErr)
	assert.False(t, status.Healthy)

	// And: health check failure metrics should be recorded with "network_error" reason
	// (timeout is treated as network error in the health checker)
	families, err := m.Gatherer().Gather()
	assert.NoError(t, err)

	found := false
	for _, family := range families {
		if family.GetName() == healthCheckFailuresMetricName {
			found = true
			assert.NotEmpty(t, family.GetMetric(), "health check failure metrics should be recorded")
		}
	}
	assert.True(t, found, healthCheckFailuresMetricName+" metric should exist")
}

// TestServerHealthCheckerCheckMetricsNilSafe tests that metrics recording is nil-safe.
func TestServerHealthCheckerCheckMetricsNilSafe(t *testing.T) {
	// Given: a healthy backend and nil metrics
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), nil)

	// When: performing a health check with nil metrics
	ctx := context.Background()
	status, err := serverChecker.Check(ctx)

	// Then: should succeed without panicking
	assert.NoError(t, err)
	assert.True(t, status.Healthy)
}

// TestServerHealthCheckerGetFailureReasonMappings tests error-to-reason mapping.
func TestServerHealthCheckerGetFailureReasonMappings(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	testCases := []struct {
		name       string
		err        error
		wantReason string
	}{
		{
			name:       "network error",
			err:        ErrHealthCheckNetworkError,
			wantReason: "network_error",
		},
		{
			name:       "unhealthy status",
			err:        ErrHealthCheckFailed,
			wantReason: "unhealthy_status",
		},
		{
			name:       "missing URL",
			err:        ErrHealthCheckURLMissing,
			wantReason: "missing_url",
		},
		{
			name:       "too many redirects",
			err:        ErrHealthCheckTooManyRedirects,
			wantReason: "too_many_redirects",
		},
		{
			name:       "nil error",
			err:        nil,
			wantReason: "unknown",
		},
		{
			name:       "unknown error",
			err:        errors.New("some random error"),
			wantReason: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := serverChecker.getFailureReason(tc.err)
			assert.Equal(t, tc.wantReason, reason)
		})
	}
}

// TestServerHealthCheckerGetFailureReasonWrappedErrors tests error mapping with wrapped errors.
func TestServerHealthCheckerGetFailureReasonWrappedErrors(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, "", newTestLogger())
	store := newMockHealthStore()
	m, err := metrics.New()
	assert.NoError(t, err)

	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger(), m)

	// Test wrapped errors
	wrappedNetworkErr := fmt.Errorf("failed to connect: %w", ErrHealthCheckNetworkError)
	reason := serverChecker.getFailureReason(wrappedNetworkErr)
	assert.Equal(t, "network_error", reason)

	wrappedStatusErr := fmt.Errorf("check failed: %w", ErrHealthCheckFailed)
	reason = serverChecker.getFailureReason(wrappedStatusErr)
	assert.Equal(t, "unhealthy_status", reason)
}
