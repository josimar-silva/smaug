package store

import (
	"sync"

	"github.com/josimar-silva/smaug/internal/health"
)

// InMemoryHealthStore implements health.HealthStore using an in-memory map with RWMutex for thread safety.
// This implementation is suitable for single-instance deployments.
// For multi-replica deployments, consider a distributed store (e.g., Redis).
type InMemoryHealthStore struct {
	mu     sync.RWMutex
	health map[string]health.ServerHealthStatus
}

// NewInMemoryHealthStore creates a new InMemoryHealthStore with an empty health state.
func NewInMemoryHealthStore() *InMemoryHealthStore {
	return &InMemoryHealthStore{
		health: make(map[string]health.ServerHealthStatus),
	}
}

// Get retrieves the current health status for a server.
// Returns (status, true) if the server has been health-checked, (zero-value, false) otherwise.
// This method is safe for concurrent reads.
func (s *InMemoryHealthStore) Get(serverID string) (health.ServerHealthStatus, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status, exists := s.health[serverID]
	return status, exists
}

// Update stores a new health status for a server.
// This method is thread-safe and will create a new entry if the server doesn't exist.
func (s *InMemoryHealthStore) Update(serverID string, status health.ServerHealthStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.health[serverID] = status
}

// GetAll returns a copy of all server health statuses.
// The returned map is a snapshot and modifications to it won't affect the store.
// This method is safe for concurrent access.
func (s *InMemoryHealthStore) GetAll() map[string]health.ServerHealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]health.ServerHealthStatus, len(s.health))
	for k, v := range s.health {
		result[k] = v
	}

	return result
}
