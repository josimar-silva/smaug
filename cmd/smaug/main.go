package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/josimar-silva/smaug/internal/config"
	"github.com/josimar-silva/smaug/internal/health"
	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	mgmhealth "github.com/josimar-silva/smaug/internal/management/health"
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/proxy"
	"github.com/josimar-silva/smaug/internal/store"
)

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

func initRouteManager(configMgr *config.ConfigManager, healthStore *store.InMemoryHealthStore, log *logger.Logger) (*proxy.RouteManager, error) {
	routeManager, err := proxy.NewRouteManager(configMgr, log, middleware.Chain(
		middleware.NewRecoveryMiddleware(log),
		middleware.NewLoggingMiddleware(log),
	), healthStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create route manager: %w", err)
	}
	if err := routeManager.Start(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to start route manager: %w", err)
	}
	return routeManager, nil
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

	routeManager, err := initRouteManager(configMgr, healthStore, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := routeManager.Stop(); err != nil {
			log.Error("failed to stop route manager", "error", err)
		}
	}()

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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Info("shutting down smaug")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
