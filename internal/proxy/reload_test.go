package proxy

import (
	"fmt"
	"testing"

	"github.com/josimar-silva/smaug/internal/config"
)

func TestRouteKey(t *testing.T) {
	tests := []struct {
		name     string
		route    config.Route
		expected string
	}{
		{
			name: "standard route",
			route: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			expected: "ollama:11434",
		},
		{
			name: "different port same name",
			route: config.Route{
				Name:     "ollama",
				Listen:   8080,
				Upstream: "http://localhost:8080",
				Server:   "saruman",
			},
			expected: "ollama:8080",
		},
		{
			name: "empty name",
			route: config.Route{
				Name:     "",
				Listen:   9000,
				Upstream: "http://localhost:9000",
				Server:   "server1",
			},
			expected: ":9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: Generating route key
			result := routeKey(tt.route)

			// Then: Should match expected key
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRouteEquals(t *testing.T) {
	tests := []struct {
		name     string
		routeA   config.Route
		routeB   config.Route
		expected bool
	}{
		{
			name: "identical routes",
			routeA: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			routeB: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			expected: true,
		},
		{
			name: "different upstream",
			routeA: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			routeB: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://newhost:11434",
				Server:   "saruman",
			},
			expected: false,
		},
		{
			name: "different server",
			routeA: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			routeB: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "gandalf",
			},
			expected: false,
		},
		{
			name: "different name",
			routeA: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			routeB: config.Route{
				Name:     "marker",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			expected: false,
		},
		{
			name: "different port",
			routeA: config.Route{
				Name:     "ollama",
				Listen:   11434,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			routeB: config.Route{
				Name:     "ollama",
				Listen:   8080,
				Upstream: "http://localhost:11434",
				Server:   "saruman",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When: Comparing routes
			result := routeEquals(tt.routeA, tt.routeB)

			// Then: Should match expected equality
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDetectChangesNoChanges(t *testing.T) {
	// Given: Identical old and new route configurations
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: Should detect no changes
	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 0 {
		t.Errorf("expected 0 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 2 {
		t.Errorf("expected 2 unchanged routes, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesAddedRoutes(t *testing.T) {
	// Given: New route added to configuration
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: Should detect one added route
	if len(changes.Added) != 1 {
		t.Fatalf("expected 1 added route, got %d", len(changes.Added))
	}
	if changes.Added[0].Name != "marker" {
		t.Errorf("expected added route 'marker', got %q", changes.Added[0].Name)
	}
	if len(changes.Removed) != 0 {
		t.Errorf("expected 0 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged route, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesRemovedRoutes(t *testing.T) {
	// Given: Route removed from configuration
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: Should detect one removed route
	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 1 {
		t.Fatalf("expected 1 removed route, got %d", len(changes.Removed))
	}
	if changes.Removed[0] != "marker:8080" {
		t.Errorf("expected removed route 'marker:8080', got %q", changes.Removed[0])
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged route, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesModifiedRoutes(t *testing.T) {
	// Given: Route with changed upstream
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://oldhost:11434",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://newhost:11434",
			Server:   "saruman",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: Should detect one modified route
	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 0 {
		t.Errorf("expected 0 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 1 {
		t.Fatalf("expected 1 modified route, got %d", len(changes.Modified))
	}
	if changes.Modified[0].Upstream != "http://newhost:11434" {
		t.Errorf("expected modified route upstream 'http://newhost:11434', got %q", changes.Modified[0].Upstream)
	}
	if len(changes.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged routes, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesMixedChanges(t *testing.T) {
	// Given: Multiple changes - add, remove, modify, unchanged
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://oldhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
		{
			Name:     "whisper",
			Listen:   9000,
			Upstream: "http://localhost:9000",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://newhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
		{
			Name:     "gemini",
			Listen:   7000,
			Upstream: "http://localhost:7000",
			Server:   "gandalf",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: Should detect all types of changes
	if len(changes.Added) != 1 {
		t.Errorf("expected 1 added route, got %d", len(changes.Added))
	} else if changes.Added[0].Name != "gemini" {
		t.Errorf("expected added route 'gemini', got %q", changes.Added[0].Name)
	}

	if len(changes.Removed) != 1 {
		t.Errorf("expected 1 removed route, got %d", len(changes.Removed))
	} else if changes.Removed[0] != "whisper:9000" {
		t.Errorf("expected removed route 'whisper:9000', got %q", changes.Removed[0])
	}

	if len(changes.Modified) != 1 {
		t.Errorf("expected 1 modified route, got %d", len(changes.Modified))
	} else if changes.Modified[0].Name != "ollama" {
		t.Errorf("expected modified route 'ollama', got %q", changes.Modified[0].Name)
	}

	if len(changes.Unchanged) != 1 {
		t.Errorf("expected 1 unchanged route, got %d", len(changes.Unchanged))
	} else if changes.Unchanged[0] != "marker:8080" {
		t.Errorf("expected unchanged route 'marker:8080', got %q", changes.Unchanged[0])
	}
}

func TestDetectChangesEmptyOldRoutes(t *testing.T) {
	// Given: Empty old routes, new routes added
	oldRoutes := []config.Route{}
	newRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: All routes should be added
	if len(changes.Added) != 2 {
		t.Errorf("expected 2 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 0 {
		t.Errorf("expected 0 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged routes, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesEmptyNewRoutes(t *testing.T) {
	// Given: All routes removed
	oldRoutes := []config.Route{
		{
			Name:     "ollama",
			Listen:   11434,
			Upstream: "http://localhost:11434",
			Server:   "saruman",
		},
		{
			Name:     "marker",
			Listen:   8080,
			Upstream: "http://localhost:8080",
			Server:   "saruman",
		},
	}
	newRoutes := []config.Route{}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: All routes should be removed
	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 2 {
		t.Errorf("expected 2 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged routes, got %d", len(changes.Unchanged))
	}
}

func TestDetectChangesBothEmpty(t *testing.T) {
	// Given: Empty old and new routes
	oldRoutes := []config.Route{}
	newRoutes := []config.Route{}

	// When: Detecting changes
	changes := detectChanges(oldRoutes, newRoutes)

	// Then: No changes should be detected
	if len(changes.Added) != 0 {
		t.Errorf("expected 0 added routes, got %d", len(changes.Added))
	}
	if len(changes.Removed) != 0 {
		t.Errorf("expected 0 removed routes, got %d", len(changes.Removed))
	}
	if len(changes.Modified) != 0 {
		t.Errorf("expected 0 modified routes, got %d", len(changes.Modified))
	}
	if len(changes.Unchanged) != 0 {
		t.Errorf("expected 0 unchanged routes, got %d", len(changes.Unchanged))
	}
}

func BenchmarkDetectChanges100Routes(b *testing.B) {
	// Given: 100 routes with 10% modified
	oldRoutes := make([]config.Route, 100)
	newRoutes := make([]config.Route, 100)

	for i := 0; i < 100; i++ {
		oldRoutes[i] = config.Route{
			Name:     fmt.Sprintf("route%d", i),
			Listen:   10000 + i,
			Upstream: fmt.Sprintf("http://oldhost:1%04d", i),
			Server:   "server1",
		}
		newRoutes[i] = config.Route{
			Name:     fmt.Sprintf("route%d", i),
			Listen:   10000 + i,
			Upstream: fmt.Sprintf("http://oldhost:1%04d", i),
			Server:   "server1",
		}
		if i%10 == 0 {
			newRoutes[i].Upstream = fmt.Sprintf("http://newhost:1%04d", i)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detectChanges(oldRoutes, newRoutes)
	}
}

func BenchmarkDetectChanges10Routes(b *testing.B) {
	// Given: 10 routes (typical homelab scenario)
	oldRoutes := make([]config.Route, 10)
	newRoutes := make([]config.Route, 10)

	for i := 0; i < 10; i++ {
		oldRoutes[i] = config.Route{
			Name:     fmt.Sprintf("route%d", i),
			Listen:   10000 + i,
			Upstream: fmt.Sprintf("http://localhost:1%04d", i),
			Server:   "server1",
		}
		newRoutes[i] = oldRoutes[i]
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detectChanges(oldRoutes, newRoutes)
	}
}
