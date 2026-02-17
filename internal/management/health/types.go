package health

// ApplicationHealth represents the overall health status of the application.
type ApplicationHealth struct {
	Status       string `json:"status"`
	Version      string `json:"version"`
	ActiveRoutes int    `json:"activeRoutes"`
	Uptime       string `json:"uptime"`
}

// LivenessResponse represents the response for the liveness probe.
// Indicates if the application is alive and running.
type LivenessResponse struct {
	Status string `json:"status"`
}

// ReadinessResponse represents the response for the readiness probe.
// Indicates if the application is ready to accept traffic.
type ReadinessResponse struct {
	Status       string `json:"status"`
	ActiveRoutes int    `json:"activeRoutes"`
}

// VersionInfo contains build version information.
type VersionInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
	GitCommit string `json:"gitCommit"`
}

// VersionResponse is an alias to VersionInfo for the version endpoint.
type VersionResponse = VersionInfo

// RouteStatusProvider defines the interface for getting route status information.
// This interface exists to decouple the health package from the proxy package.
type RouteStatusProvider interface {
	// GetActiveRouteCount returns the number of currently active routes.
	GetActiveRouteCount() int
}
