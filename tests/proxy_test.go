//go:build integration

package tests

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/proxy"
	"github.com/josimar-silva/smaug/internal/store"
)

func TestIntegrationProxyHotReloadAddRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service1",
				Listen:   getFreePort(t),
				Upstream: backend1.URL,
				Server:   "server1",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
		},
	}

	writeConfig(t, configPath, initialConfig)

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	defer configMgr.Stop()

	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	routeManager, err := proxy.NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	if err := routeManager.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer routeManager.Stop()

	time.Sleep(100 * time.Millisecond)

	routes := routeManager.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	port2 := getFreePort(t)
	newConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service1",
				Listen:   initialConfig.Routes[0].Listen,
				Upstream: backend1.URL,
				Server:   "server1",
			},
			{
				Name:     "service2",
				Listen:   port2,
				Upstream: backend2.URL,
				Server:   "server2",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
			"server2": {},
		},
	}

	writeConfig(t, configPath, newConfig)

	time.Sleep(2 * time.Second)

	routes = routeManager.GetActiveRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes after reload, got %d", len(routes))
	}

	routeNames := make(map[string]bool)
	for _, route := range routes {
		routeNames[route.Name] = true
	}

	if !routeNames["service1"] || !routeNames["service2"] {
		t.Errorf("expected both service1 and service2 to be active")
	}
}

func TestIntegrationProxyHotReloadRemoveRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	port1 := getFreePort(t)
	port2 := getFreePort(t)

	initialConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service1",
				Listen:   port1,
				Upstream: backend1.URL,
				Server:   "server1",
			},
			{
				Name:     "service2",
				Listen:   port2,
				Upstream: backend2.URL,
				Server:   "server2",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
			"server2": {},
		},
	}

	writeConfig(t, configPath, initialConfig)

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	defer configMgr.Stop()

	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	routeManager, err := proxy.NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	if err := routeManager.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer routeManager.Stop()

	time.Sleep(100 * time.Millisecond)

	routes := routeManager.GetActiveRoutes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	newConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service1",
				Listen:   port1,
				Upstream: backend1.URL,
				Server:   "server1",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
		},
	}

	writeConfig(t, configPath, newConfig)

	time.Sleep(2 * time.Second)

	routes = routeManager.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after reload, got %d", len(routes))
	}

	if routes[0].Name != "service1" {
		t.Errorf("expected service1 to remain, got %s", routes[0].Name)
	}
}

func TestIntegrationProxyHotReloadModifyRoute(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend2"))
	}))
	defer backend2.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	port := getFreePort(t)

	initialConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service",
				Listen:   port,
				Upstream: backend1.URL,
				Server:   "server1",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
		},
	}

	writeConfig(t, configPath, initialConfig)

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	defer configMgr.Stop()

	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	routeManager, err := proxy.NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	if err := routeManager.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer routeManager.Stop()

	time.Sleep(100 * time.Millisecond)

	routes := routeManager.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Upstream != backend1.URL {
		t.Errorf("expected upstream %s, got %s", backend1.URL, routes[0].Upstream)
	}

	newConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service",
				Listen:   port,
				Upstream: backend2.URL,
				Server:   "server1",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
		},
	}

	writeConfig(t, configPath, newConfig)

	time.Sleep(2 * time.Second)

	routes = routeManager.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after reload, got %d", len(routes))
	}
	if routes[0].Upstream != backend2.URL {
		t.Errorf("expected upstream %s after reload, got %s", backend2.URL, routes[0].Upstream)
	}
}

func TestIntegrationProxyHotReloadRapidChanges(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("backend"))
	}))
	defer backend.Close()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialConfig := &config.Config{
		Routes: []config.Route{
			{
				Name:     "service",
				Listen:   getFreePort(t),
				Upstream: backend.URL,
				Server:   "server1",
			},
		},
		Servers: map[string]config.Server{
			"server1": {},
		},
	}

	writeConfig(t, configPath, initialConfig)

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		t.Fatalf("failed to create config manager: %v", err)
	}
	defer configMgr.Stop()

	mw := middleware.Chain()
	healthStore := store.NewInMemoryHealthStore()

	routeManager, err := proxy.NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		t.Fatalf("failed to create route manager: %v", err)
	}

	if err := routeManager.Start(ctx); err != nil {
		t.Fatalf("failed to start route manager: %v", err)
	}
	defer routeManager.Stop()

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 5; i++ {
		newConfig := &config.Config{
			Routes: []config.Route{
				{
					Name:     "service",
					Listen:   initialConfig.Routes[0].Listen,
					Upstream: backend.URL,
					Server:   "server1",
				},
			},
			Servers: map[string]config.Server{
				"server1": {},
			},
		}

		writeConfig(t, configPath, newConfig)
		time.Sleep(500 * time.Millisecond)
	}

	time.Sleep(2 * time.Second)

	routes := routeManager.GetActiveRoutes()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after rapid changes, got %d", len(routes))
	}
}

func writeConfig(t *testing.T, path string, cfg *config.Config) {
	t.Helper()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("failed to get TCP address")
	}

	return addr.Port
}
