package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/josimar-silva/smaug/internal/client/gwaihir"
	sleepclient "github.com/josimar-silva/smaug/internal/client/sleep"
	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/health"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
	mgmhealth "github.com/josimar-silva/smaug/internal/management/health"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/proxy"
	"github.com/josimar-silva/smaug/internal/store"
)

// defaultGwaihirTimeout is used when GwaihirConfig.Timeout is zero or not set.
const defaultGwaihirTimeout = 5 * time.Second

// defaultIdleCheckInterval is the period between idle checks in the IdleTracker.
const defaultIdleCheckInterval = time.Minute

// gracefulShutdownTimeout is the maximum duration to wait for in-flight requests to complete
// during graceful shutdown. This aligns with Kubernetes pod termination grace period.
const gracefulShutdownTimeout = 30 * time.Second

// routeSleepSender implements proxy.SleepSender by dispatching sleep commands to
// the correct upstream endpoint based on the route identifier.
//
// Each route maps to a server, and each server has its own SleepOnLan endpoint.
// This adapter is built once at startup from config and passed to SleepCoordinator.
type routeSleepSender struct {
	// senders maps route name to the sleep client for that route's server.
	senders map[string]*sleepclient.Client
	log     *logger.Logger
}

// Sleep sends a sleep command to the upstream endpoint associated with routeID.
// If no sender is registered for the route, the command is a no-op (logged at warn level).
func (r *routeSleepSender) Sleep(ctx context.Context, routeID string) error {
	client, ok := r.senders[routeID]
	if !ok {
		r.log.WarnContext(ctx, "no sleep client registered for route; skipping",
			"operation", "route_sleep_sender",
			"route_id", routeID,
		)
		return nil
	}

	return client.Sleep(ctx)
}

func initLogger(cfg *config.Config, oldLog *logger.Logger) *logger.Logger {
	log := logger.New(
		logger.LevelFrom(cfg.Settings.Logging.Level),
		logger.FormatFrom(cfg.Settings.Logging.Format),
		nil,
	)
	if err := oldLog.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to stop original logger: %v\n", err)
	}
	return log
}

func initMetrics(log *logger.Logger) (*metrics.Registry, error) {
	m, err := metrics.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metrics: %w", err)
	}
	log.Info("metrics registry initialized", "operation", "init_metrics")
	return m, nil
}

func initConfigManager(configPath string, log *logger.Logger) (*config.ConfigManager, error) {
	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create config manager: %w", err)
	}
	return configMgr, nil
}

func initHealthManager(cfg *config.Config, healthStore *store.InMemoryHealthStore, log *logger.Logger) (*health.HealthManager, error) {
	healthManager := health.NewHealthManager(cfg, healthStore, log)
	if err := healthManager.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start health manager: %w", err)
	}
	return healthManager, nil
}

func initIdleTracker(cfg *config.Config, log *logger.Logger) (*proxy.IdleTracker, error) {
	solCount := 0
	for _, route := range cfg.Routes {
		if server, ok := cfg.Servers[route.Server]; ok && server.SleepOnLan.Enabled {
			solCount++
		}
	}

	if solCount == 0 {
		log.Info("no routes with Sleep-on-LAN enabled; idle tracking disabled",
			"operation", "init_idle_tracker",
		)
		return nil, nil
	}

	idleTracker, err := proxy.NewIdleTracker(proxy.IdleTrackerConfig{
		CheckInterval: defaultIdleCheckInterval,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create idle tracker: %w", err)
	}

	for _, route := range cfg.Routes {
		server, ok := cfg.Servers[route.Server]
		if !ok || !server.SleepOnLan.Enabled {
			continue
		}
		idleTracker.RegisterRoute(route.Name, server.SleepOnLan.IdleTimeout)
	}

	log.Info("Sleep-on-LAN idle tracking enabled",
		"operation", "init_idle_tracker",
		"route_count", solCount,
	)

	return idleTracker, nil
}

// initSleepCoordinator creates a SleepCoordinator that listens on idleTracker's
// sleep triggers channel and sends sleep commands to each server's configured endpoint.
//
// Returns nil (no error) when there are no SleepOnLan-enabled routes or when
// idleTracker is nil — the caller is responsible for not starting a nil coordinator.
func initSleepCoordinator(cfg *config.Config, idleTracker *proxy.IdleTracker, log *logger.Logger) (*proxy.SleepCoordinator, error) {
	if idleTracker == nil {
		return nil, nil
	}

	senders := make(map[string]*sleepclient.Client)

	for _, route := range cfg.Routes {
		server, ok := cfg.Servers[route.Server]
		if !ok || !server.SleepOnLan.Enabled {
			continue
		}

		if server.SleepOnLan.Endpoint == "" {
			log.Warn("Sleep-on-LAN endpoint not configured for route; skipping",
				"operation", "init_sleep_coordinator",
				"route", route.Name,
				"server", route.Server,
			)
			continue
		}

		clientCfg := sleepclient.NewClientConfig(server.SleepOnLan.Endpoint)
		client, err := sleepclient.NewClient(clientCfg, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create sleep client for route %q (server %q): %w",
				route.Name, route.Server, err)
		}

		senders[route.Name] = client

		log.Info("sleep client configured for route",
			"operation", "init_sleep_coordinator",
			"route", route.Name,
			"server", route.Server,
		)
	}

	if len(senders) == 0 {
		log.Info("no sleep clients configured; sleep coordination disabled",
			"operation", "init_sleep_coordinator",
		)
		return nil, nil
	}

	sender := &routeSleepSender{senders: senders, log: log}

	coordinator, err := proxy.NewSleepCoordinator(idleTracker.SleepTriggers(), sender, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create sleep coordinator: %w", err)
	}

	log.Info("sleep coordination enabled",
		"operation", "init_sleep_coordinator",
		"route_count", len(senders),
	)

	return coordinator, nil
}

func initRouteManager(configMgr *config.ConfigManager, healthStore *store.InMemoryHealthStore, log *logger.Logger, wakeOpts *proxy.WakeOptions, idleTracker proxy.ActivityRecorder) (*proxy.RouteManager, error) {
	routeManager, err := proxy.NewRouteManager(configMgr, log, middleware.Chain(
		middleware.NewRecoveryMiddleware(log),
		middleware.NewLoggingMiddleware(log),
	), healthStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create route manager: %w", err)
	}

	if wakeOpts != nil {
		routeManager.SetWakeOptions(*wakeOpts)
	}

	if idleTracker != nil {
		routeManager.SetIdleTracker(idleTracker)
	}

	if err := routeManager.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start route manager: %w", err)
	}
	return routeManager, nil
}

func initWakeOptions(cfg *config.Config, log *logger.Logger) (*proxy.WakeOptions, error) {
	if cfg.Settings.Gwaihir.URL == "" {
		log.Info("Gwaihir URL not configured; Wake-on-LAN coordination disabled")
		return nil, nil
	}

	wolCount := 0
	for _, serverCfg := range cfg.Servers {
		if serverCfg.WakeOnLan.Enabled {
			wolCount++
		}
	}
	if wolCount == 0 {
		log.Info("no servers with Wake-on-LAN enabled; wake coordination disabled")
		return nil, nil
	}

	timeout := cfg.Settings.Gwaihir.Timeout
	if timeout <= 0 {
		timeout = defaultGwaihirTimeout
	}

	wolClient, err := gwaihir.NewClient(gwaihir.ClientConfig{
		BaseURL:     cfg.Settings.Gwaihir.URL,
		APIKey:      cfg.Settings.Gwaihir.APIKey.Value(),
		Timeout:     timeout,
		RetryConfig: gwaihir.NewRetryConfig(),
	}, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create Gwaihir client: %w", err)
	}

	log.Info("Wake-on-LAN coordination enabled", "server_count", wolCount)

	return &proxy.WakeOptions{Sender: wolClient}, nil
}

func startManagementServer(cfg *config.Config, routeStatusProvider mgmhealth.RouteStatusProvider, log *logger.Logger) (*mgmhealth.Server, error) {
	versionInfo := mgmhealth.VersionInfo{
		Version:   Version,
		BuildTime: BuildTime,
		GitCommit: GitCommit,
	}

	managementServer := mgmhealth.NewServer(
		cfg.Settings.Observability.HealthCheck.Port,
		routeStatusProvider,
		versionInfo,
		log,
	)

	if err := managementServer.Start(context.Background()); err != nil {
		return nil, err
	}

	log.Info("management server started",
		"port", cfg.Settings.Observability.HealthCheck.Port,
	)

	return managementServer, nil
}

func run() error {
	configPath := os.Getenv("SMAUG_CONFIG")
	if configPath == "" {
		configPath = "/etc/smaug/services.yaml"
	}

	log := logger.NewFromEnvs()
	defer func() {
		if err := log.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to stop logger: %v\n", err)
		}
	}()

	configMgr, err := initConfigManager(configPath, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := configMgr.Stop(); err != nil {
			log.Error("failed to stop config manager", "error", err)
		}
	}()

	cfg := configMgr.GetConfig()
	log = initLogger(cfg, log)

	log.Info("starting smaug", "config_path", configPath)

	// Initialize metrics
	m, err := initMetrics(log)
	if err != nil {
		return err
	}

	healthStore := store.NewInMemoryHealthStore()

	healthManager, err := initHealthManager(cfg, healthStore, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := healthManager.Stop(); err != nil {
			log.Error("failed to stop health manager", "error", err)
		}
	}()

	// Wire metrics into health manager
	healthManager.SetMetrics(m)

	wakeOpts, err := initWakeOptions(cfg, log)
	if err != nil {
		return err
	}

	// Wire metrics into Gwaihir client
	if wakeOpts != nil && wakeOpts.Sender != nil {
		if client, ok := wakeOpts.Sender.(*gwaihir.Client); ok {
			client.SetMetrics(m)
		}
	}

	idleTracker, err := initIdleTracker(cfg, log)
	if err != nil {
		return err
	}

	// Wire metrics into idle tracker
	if idleTracker != nil {
		idleTracker.SetMetrics(m)
	}

	var shutdownCtx context.Context
	var shutdownCancel context.CancelFunc

	shutdownCtx = context.Background()

	if idleTracker != nil {
		if err := idleTracker.Start(shutdownCtx); err != nil {
			return fmt.Errorf("failed to start idle tracker: %w", err)
		}
		defer func() {
			if err := idleTracker.Stop(); err != nil {
				log.Error("failed to stop idle tracker", "error", err)
			}
		}()
	}

	routeManager, err := initRouteManager(configMgr, healthStore, log, wakeOpts, idleTracker)
	if err != nil {
		return err
	}
	defer func() {
		if err := routeManager.Stop(); err != nil {
			log.Error("failed to stop route manager", "error", err)
		}
	}()

	// Wire metrics into route manager
	routeManager.SetMetrics(m)

	// Wire metrics into config manager
	configMgr.SetMetrics(m)

	sleepCoordinator, err := initSleepCoordinator(cfg, idleTracker, log)
	if err != nil {
		return err
	}

	if sleepCoordinator != nil {
		if err := sleepCoordinator.Start(shutdownCtx); err != nil {
			return fmt.Errorf("failed to start sleep coordinator: %w", err)
		}
		defer func() {
			if err := sleepCoordinator.Stop(); err != nil {
				log.Error("failed to stop sleep coordinator", "error", err)
			}
		}()
	}

	if cfg.Settings.Observability.HealthCheck.Enabled {
		managementServer, err := startManagementServer(cfg, routeManager, log)
		if err != nil {
			return fmt.Errorf("failed to start management server: %w", err)
		}
		defer func() {
			if err := managementServer.Stop(); err != nil {
				log.Error("failed to stop management server", "error", err)
			}
		}()
	}

	log.Info("smaug started successfully")

	shutdownCtx, shutdownCancel = context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer shutdownCancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan

	log.InfoContext(shutdownCtx, "shutdown signal received, starting graceful shutdown",
		"signal", sig.String(),
		"timeout", gracefulShutdownTimeout,
	)

	shutdownCancel()

	log.Info("graceful shutdown completed")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
