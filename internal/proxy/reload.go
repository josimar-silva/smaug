package proxy

import (
	"fmt"

	"github.com/josimar-silva/smaug/internal/config"
)

// RouteChanges represents the diff between old and new route configurations.
type RouteChanges struct {
	Added     []config.Route
	Removed   []string
	Modified  []config.Route
	Unchanged []string
}

// routeKey generates a unique key for a route based on name and port.
func routeKey(route config.Route) string {
	return fmt.Sprintf("%s:%d", route.Name, route.Listen)
}

// routeEquals checks if two routes are functionally equivalent.
func routeEquals(a, b config.Route) bool {
	return a.Name == b.Name &&
		a.Listen == b.Listen &&
		a.Upstream == b.Upstream &&
		a.Server == b.Server
}

// detectChanges computes the diff between old and new route configurations.
func detectChanges(oldRoutes, newRoutes []config.Route) RouteChanges {
	changes := RouteChanges{
		Added:     make([]config.Route, 0),
		Removed:   make([]string, 0),
		Modified:  make([]config.Route, 0),
		Unchanged: make([]string, 0),
	}

	oldMap := make(map[string]config.Route)
	newMap := make(map[string]config.Route)

	for _, route := range oldRoutes {
		key := routeKey(route)
		oldMap[key] = route
	}

	for _, route := range newRoutes {
		key := routeKey(route)
		newMap[key] = route
	}

	for key, newRoute := range newMap {
		if oldRoute, exists := oldMap[key]; exists {
			if !routeEquals(oldRoute, newRoute) {
				changes.Modified = append(changes.Modified, newRoute)
			} else {
				changes.Unchanged = append(changes.Unchanged, key)
			}
		} else {
			changes.Added = append(changes.Added, newRoute)
		}
	}

	for key := range oldMap {
		if _, exists := newMap[key]; !exists {
			changes.Removed = append(changes.Removed, key)
		}
	}

	return changes
}
