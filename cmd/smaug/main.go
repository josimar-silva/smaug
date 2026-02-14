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
	"github.com/josimar-silva/smaug/internal/middleware"
	"github.com/josimar-silva/smaug/internal/proxy"
	"github.com/josimar-silva/smaug/internal/store"
)

func run() error {
	configPath := os.Getenv("SMAUG_CONFIG")
	if configPath == "" {
		configPath = "/etc/smaug/services.yaml"
	}

	log := logger.New(logger.LevelInfo, logger.JSON, nil)
	log.Info("starting smaug", "config_path", configPath)

	configMgr, err := config.NewManager(configPath, log)
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}
	defer func() {
		if err := configMgr.Stop(); err != nil {
			log.Error("failed to stop config manager", "error", err)
		}
	}()

	cfg := configMgr.GetConfig()
	healthStore := store.NewInMemoryHealthStore()

	healthManager := health.NewHealthManager(cfg, healthStore, log)
	if err := healthManager.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start health manager: %w", err)
	}
	defer func() {
		if err := healthManager.Stop(); err != nil {
			log.Error("failed to stop health manager", "error", err)
		}
	}()

	mw := middleware.Chain(
		middleware.NewRecoveryMiddleware(log),
		middleware.NewLoggingMiddleware(log),
	)

	routeManager, err := proxy.NewRouteManager(configMgr, log, mw, healthStore)
	if err != nil {
		return fmt.Errorf("failed to create route manager: %w", err)
	}

	if err := routeManager.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start route manager: %w", err)
	}
	defer func() {
		if err := routeManager.Stop(); err != nil {
			log.Error("failed to stop route manager", "error", err)
		}
	}()

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
