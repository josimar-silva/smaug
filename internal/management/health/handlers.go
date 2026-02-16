package health

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
)

const (
	errMethodNotAllowed = "Method not allowed"
	contentTypeJSON     = "application/json"
	headerContentType   = "Content-Type"
)

// NewHealthHandler creates an HTTP handler for the /health endpoint.
// Returns overall application health status including version, active routes, and uptime.
func NewHealthHandler(provider RouteStatusProvider, versionInfo VersionInfo, log *logger.Logger, startTime time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}

		uptime := time.Since(startTime).Round(time.Second)

		response := ApplicationHealth{
			Status:       "healthy",
			Version:      versionInfo.Version,
			ActiveRoutes: provider.GetActiveRouteCount(),
			Uptime:       uptime.String(),
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error("failed to encode health response", "error", err)
		}
	})
}

// NewLiveHandler creates an HTTP handler for the /live endpoint.
// This is a Kubernetes liveness probe that indicates the application is alive.
func NewLiveHandler(log *logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}

		response := LivenessResponse{
			Status: "alive",
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error("failed to encode liveness response", "error", err)
		}
	})
}

// NewReadyHandler creates an HTTP handler for the /ready endpoint.
// This is a Kubernetes readiness probe that indicates the application is ready to accept traffic.
// Returns 503 Service Unavailable if no routes are active.
func NewReadyHandler(provider RouteStatusProvider, log *logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}

		activeRoutes := provider.GetActiveRouteCount()
		status := "ready"
		statusCode := http.StatusOK

		if activeRoutes == 0 {
			status = "not ready"
			statusCode = http.StatusServiceUnavailable
		}

		response := ReadinessResponse{
			Status:       status,
			ActiveRoutes: activeRoutes,
		}

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(statusCode)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error("failed to encode readiness response", "error", err)
		}
	})
}

// NewVersionHandler creates an HTTP handler for the /version endpoint.
// Returns build version information including version, build time, and git commit.
func NewVersionHandler(versionInfo VersionInfo, log *logger.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, errMethodNotAllowed, http.StatusMethodNotAllowed)
			return
		}

		response := VersionResponse(versionInfo)

		w.Header().Set(headerContentType, contentTypeJSON)
		w.WriteHeader(http.StatusOK)

		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Error("failed to encode version response", "error", err)
		}
	})
}
