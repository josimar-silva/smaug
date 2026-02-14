package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

type mockHealthStore struct {
	statuses map[string]ServerHealthStatus
}

func newMockHealthStore() *mockHealthStore {
	return &mockHealthStore{
		statuses: make(map[string]ServerHealthStatus),
	}
}

func (m *mockHealthStore) Get(serverID string) ServerHealthStatus {
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
	m.statuses[serverID] = status
}

func (m *mockHealthStore) GetAll() map[string]ServerHealthStatus {
	result := make(map[string]ServerHealthStatus, len(m.statuses))
	for k, v := range m.statuses {
		result[k] = v
	}
	return result
}

// TestNewServerHealthChecker_ValidParameters tests successful construction.
func TestNewServerHealthChecker_ValidParameters(t *testing.T) {
	// Given: valid parameters
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()

	// When: creating a ServerHealthChecker
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

	// Then: should create successfully
	assert.NotNil(t, serverChecker)
	assert.Equal(t, "saruman", serverChecker.serverID)
	assert.NotNil(t, serverChecker.checker)
	assert.NotNil(t, serverChecker.store)
	assert.NotNil(t, serverChecker.logger)
}

// TestNewServerHealthChecker_PanicsOnInvalidParameters tests constructor validation.
func TestNewServerHealthChecker_PanicsOnInvalidParameters(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
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
					NewServerHealthChecker(tc.serverID, tc.checker, tc.store, tc.logger)
				})
			} else {
				assert.NotPanics(t, func() {
					NewServerHealthChecker(tc.serverID, tc.checker, tc.store, tc.logger)
				})
			}
		})
	}
}

// TestServerHealthChecker_Check_Success tests successful health check.
func TestServerHealthChecker_Check_Success(t *testing.T) {
	// Given: a healthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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

// TestServerHealthChecker_Check_Failure tests failed health check.
func TestServerHealthChecker_Check_Failure(t *testing.T) {
	// Given: an unhealthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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

// TestServerHealthChecker_Check_NetworkError tests network failure handling.
func TestServerHealthChecker_Check_NetworkError(t *testing.T) {
	// Given: an unreachable endpoint
	checker := NewHealthChecker("http://192.0.2.1:9999/health", 100*time.Millisecond, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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

// TestServerHealthChecker_Check_ContextCancellation tests cancellation handling.
func TestServerHealthChecker_Check_ContextCancellation(t *testing.T) {
	// Given: a slow backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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

// TestServerHealthChecker_Check_UpdatesTimestamp tests that timestamps are updated.
func TestServerHealthChecker_Check_UpdatesTimestamp(t *testing.T) {
	// Given: a healthy backend
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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

// TestServerHealthChecker_Check_TransitionFromHealthyToUnhealthy tests state transitions.
func TestServerHealthChecker_Check_TransitionFromHealthyToUnhealthy(t *testing.T) {
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

	checker := NewHealthChecker(backend.URL, 2*time.Second, newTestLogger())
	store := newMockHealthStore()
	serverChecker := NewServerHealthChecker("saruman", checker, store, newTestLogger())

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
