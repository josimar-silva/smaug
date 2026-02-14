package store

import (
	"sync"
	"testing"
	"time"

	"github.com/josimar-silva/smaug/internal/health"
	"github.com/stretchr/testify/assert"
)

// TestInMemoryHealthStore_Get_UnknownServer tests that Get returns unhealthy default for unknown servers.
func TestInMemoryHealthStore_Get_UnknownServer(t *testing.T) {
	// Given: an empty health store
	store := NewInMemoryHealthStore()

	// When: getting status for a server that has never been checked
	status := store.Get("unknown-server")

	// Then: should return default unhealthy status
	assert.Equal(t, "unknown-server", status.ServerID)
	assert.False(t, status.Healthy, "unknown server should be unhealthy by default")
	assert.Empty(t, status.LastError)
	assert.True(t, status.LastCheckedAt.IsZero(), "timestamp should be zero value")
}

// TestInMemoryHealthStore_Update_NewServer tests updating a new server's health status.
func TestInMemoryHealthStore_Update_NewServer(t *testing.T) {
	// Given: an empty health store
	store := NewInMemoryHealthStore()
	now := time.Now()

	// When: updating health status for a new server
	status := health.ServerHealthStatus{
		ServerID:      "saruman",
		Healthy:       true,
		LastCheckedAt: now,
		LastError:     "",
	}
	store.Update("saruman", status)

	// Then: the status should be retrievable
	retrieved := store.Get("saruman")
	assert.Equal(t, "saruman", retrieved.ServerID)
	assert.True(t, retrieved.Healthy)
	assert.Equal(t, now, retrieved.LastCheckedAt)
	assert.Empty(t, retrieved.LastError)
}

// TestInMemoryHealthStore_Update_ExistingServer tests that Update overwrites existing status.
func TestInMemoryHealthStore_Update_ExistingServer(t *testing.T) {
	// Given: a store with an existing server status
	store := NewInMemoryHealthStore()
	firstTime := time.Now()
	store.Update("saruman", health.ServerHealthStatus{
		ServerID:      "saruman",
		Healthy:       true,
		LastCheckedAt: firstTime,
		LastError:     "",
	})

	// When: updating the same server with a new status
	secondTime := firstTime.Add(5 * time.Second)
	store.Update("saruman", health.ServerHealthStatus{
		ServerID:      "saruman",
		Healthy:       false,
		LastCheckedAt: secondTime,
		LastError:     "connection refused",
	})

	// Then: the new status should overwrite the old one
	retrieved := store.Get("saruman")
	assert.Equal(t, "saruman", retrieved.ServerID)
	assert.False(t, retrieved.Healthy, "should have updated to unhealthy")
	assert.Equal(t, secondTime, retrieved.LastCheckedAt)
	assert.Equal(t, "connection refused", retrieved.LastError)
}

// TestInMemoryHealthStore_GetAll_EmptyStore tests GetAll on an empty store.
func TestInMemoryHealthStore_GetAll_EmptyStore(t *testing.T) {
	// Given: an empty health store
	store := NewInMemoryHealthStore()

	// When: getting all statuses
	all := store.GetAll()

	// Then: should return an empty map
	assert.NotNil(t, all, "should return non-nil map")
	assert.Empty(t, all, "should be empty")
}

// TestInMemoryHealthStore_GetAll_MultipleServers tests GetAll with multiple servers.
func TestInMemoryHealthStore_GetAll_MultipleServers(t *testing.T) {
	// Given: a store with multiple servers
	store := NewInMemoryHealthStore()
	now := time.Now()

	servers := map[string]health.ServerHealthStatus{
		"saruman": {
			ServerID:      "saruman",
			Healthy:       true,
			LastCheckedAt: now,
			LastError:     "",
		},
		"morgoth": {
			ServerID:      "morgoth",
			Healthy:       false,
			LastCheckedAt: now.Add(-1 * time.Minute),
			LastError:     "timeout",
		},
		"sauron": {
			ServerID:      "sauron",
			Healthy:       true,
			LastCheckedAt: now.Add(-30 * time.Second),
			LastError:     "",
		},
	}

	for id, status := range servers {
		store.Update(id, status)
	}

	// When: getting all statuses
	all := store.GetAll()

	// Then: should return all servers with correct statuses
	assert.Len(t, all, 3)
	for id, expected := range servers {
		actual, exists := all[id]
		assert.True(t, exists, "server %s should exist in GetAll result", id)
		assert.Equal(t, expected.ServerID, actual.ServerID)
		assert.Equal(t, expected.Healthy, actual.Healthy)
		assert.Equal(t, expected.LastCheckedAt, actual.LastCheckedAt)
		assert.Equal(t, expected.LastError, actual.LastError)
	}
}

// TestInMemoryHealthStore_GetAll_ReturnsSnapshot tests that GetAll returns a copy.
func TestInMemoryHealthStore_GetAll_ReturnsSnapshot(t *testing.T) {
	// Given: a store with a server
	store := NewInMemoryHealthStore()
	store.Update("saruman", health.ServerHealthStatus{
		ServerID: "saruman",
		Healthy:  true,
	})

	// When: getting all statuses and modifying the returned map
	all := store.GetAll()
	all["saruman"] = health.ServerHealthStatus{
		ServerID: "saruman",
		Healthy:  false, // Attempt to modify
	}
	all["new-server"] = health.ServerHealthStatus{
		ServerID: "new-server",
		Healthy:  true,
	}

	// Then: the store should not be affected
	storedStatus := store.Get("saruman")
	assert.True(t, storedStatus.Healthy, "store should still have original status")

	newServerStatus := store.Get("new-server")
	assert.False(t, newServerStatus.Healthy, "new server should not exist in store")
}

// TestInMemoryHealthStore_ConcurrentReads tests that concurrent reads don't cause data races.
func TestInMemoryHealthStore_ConcurrentReads(t *testing.T) {
	// Given: a store with multiple servers
	store := NewInMemoryHealthStore()
	for i := 0; i < 10; i++ {
		store.Update("server"+string(rune('0'+i)), health.ServerHealthStatus{
			ServerID: "server" + string(rune('0'+i)),
			Healthy:  i%2 == 0,
		})
	}

	// When: performing concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			serverID := "server" + string(rune('0'+n%10))
			_ = store.Get(serverID)
		}(i)
	}

	// Then: all goroutines should complete without panics
	wg.Wait()
}

// TestInMemoryHealthStore_ConcurrentWrites tests that concurrent writes are safe.
func TestInMemoryHealthStore_ConcurrentWrites(t *testing.T) {
	// Given: an empty store
	store := NewInMemoryHealthStore()

	// When: performing concurrent writes to different servers
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			serverID := "server" + string(rune('0'+n%10))
			store.Update(serverID, health.ServerHealthStatus{
				ServerID: serverID,
				Healthy:  n%2 == 0,
			})
		}(i)
	}

	wg.Wait()

	// Then: all servers should exist in the store
	all := store.GetAll()
	assert.Len(t, all, 10, "should have 10 unique servers")
}

// TestInMemoryHealthStore_ConcurrentReadWrite tests concurrent reads and writes.
func TestInMemoryHealthStore_ConcurrentReadWrite(t *testing.T) {
	// Given: a store with some initial data
	store := NewInMemoryHealthStore()
	for i := 0; i < 5; i++ {
		store.Update("server"+string(rune('0'+i)), health.ServerHealthStatus{
			ServerID: "server" + string(rune('0'+i)),
			Healthy:  true,
		})
	}

	// When: performing concurrent reads and writes
	var wg sync.WaitGroup

	// Readers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				serverID := "server" + string(rune('0'+j%5))
				_ = store.Get(serverID)
			}
		}()
	}

	// Writers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				serverID := "server" + string(rune('0'+j%5))
				store.Update(serverID, health.ServerHealthStatus{
					ServerID: serverID,
					Healthy:  n%2 == 0,
				})
			}
		}(i)
	}

	// Then: all operations should complete without data races
	wg.Wait()

	// Verify final state is consistent
	all := store.GetAll()
	assert.Len(t, all, 5)
	for _, status := range all {
		assert.NotEmpty(t, status.ServerID)
	}
}

// TestInMemoryHealthStore_GetAll_ConcurrentAccess tests GetAll during concurrent modifications.
func TestInMemoryHealthStore_GetAll_ConcurrentAccess(t *testing.T) {
	// Given: a store being modified concurrently
	store := NewInMemoryHealthStore()
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				store.Update("server"+string(rune('0'+n)), health.ServerHealthStatus{
					ServerID: "server" + string(rune('0'+n)),
					Healthy:  j%2 == 0,
				})
			}
		}(i)
	}

	// Readers calling GetAll
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				all := store.GetAll()
				_ = all // Use the result
			}
		}()
	}

	// Then: all operations should complete without races
	wg.Wait()
}
