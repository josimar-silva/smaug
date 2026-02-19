package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/josimar-silva/smaug/internal/infrastructure/logger"
	"github.com/josimar-silva/smaug/internal/infrastructure/metrics"
)

// ConfigManager handles configuration loading and hot-reloading.
// It watches a configuration file for changes and automatically reloads
// the configuration when the file is modified, providing thread-safe
// access to the current configuration state.
type ConfigManager struct {
	config   atomic.Value
	path     string
	watcher  *fsnotify.Watcher
	reloadCh chan struct{}
	done     chan struct{}
	wg       sync.WaitGroup
	stopOnce sync.Once
	log      *logger.Logger
	metrics  *metrics.Registry // Metrics registry (optional)
}

// NewManager creates a new ConfigManager and starts watching the config file.
// It performs an initial load of the configuration file and sets up file watching
// for automatic reloads when the file changes.
//
// Parameters:
//   - path: The absolute or relative path to the YAML configuration file
//   - log: The logger instance to use for logging config operations
//
// Returns:
//   - A pointer to the initialized ConfigManager
//   - An error if the initial load fails, watcher creation fails, or watch setup fails
//
// The returned ConfigManager must be cleaned up by calling Stop() when no longer needed.
func NewManager(path string, log *logger.Logger) (*ConfigManager, error) {
	config, err := Load(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config %s: %w", path, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	cm := &ConfigManager{
		path:     path,
		watcher:  watcher,
		reloadCh: make(chan struct{}, 1),
		done:     make(chan struct{}),
		log:      log,
	}
	cm.config.Store(config)

	configDir := filepath.Dir(path)
	if err := cm.watcher.Add(configDir); err != nil {
		_ = cm.watcher.Close()
		return nil, fmt.Errorf("failed to add watcher for %s: %w", configDir, err)
	}

	cm.wg.Add(1)
	go cm.watch()

	return cm, nil
}

// SetMetrics sets the metrics registry for recording config reload outcomes.
// Should be called before the manager is actively used.
func (cm *ConfigManager) SetMetrics(reg *metrics.Registry) {
	cm.metrics = reg
}

// GetConfig returns the current configuration in a thread-safe manner.
// The configuration is accessed using lock-free atomic operations.
//
// Returns:
//   - A pointer to the current Config, or nil if no config has been loaded
//
// The returned Config pointer is safe to read concurrently but should not be modified.
// If the config is reloaded, this method will return the new config on subsequent calls.
func (cm *ConfigManager) GetConfig() *Config {
	val := cm.config.Load()
	if val == nil {
		return nil
	}
	cfg, ok := val.(*Config)
	if !ok {
		return nil
	}
	return cfg
}

// ReloadSignal returns a channel that receives a signal when the config is reloaded.
// The channel is buffered with capacity 1, so it will not block the reload process
// if no listener is actively reading from it.
//
// Returns:
//   - A receive-only channel that signals config reload events
//
// Consumers should read from this channel in a select statement to avoid blocking.
// If multiple reloads happen rapidly, only one signal may be received.
func (cm *ConfigManager) ReloadSignal() <-chan struct{} {
	return cm.reloadCh
}

// Stop stops the configuration watcher and waits for all goroutines to finish.
// This method is idempotent and safe to call multiple times.
//
// Returns:
//   - An error if closing the watcher fails, nil otherwise
//
// After Stop returns, no further config reloads will occur and all background
// goroutines have exited. Any in-flight reload operations will be cancelled.
func (cm *ConfigManager) Stop() error {
	var err error
	cm.stopOnce.Do(func() {
		close(cm.done)
		cm.wg.Wait()
		err = cm.watcher.Close()
	})
	return err
}

func (cm *ConfigManager) watch() {
	defer cm.wg.Done()

	fileName := filepath.Base(cm.path)
	var timer *time.Timer
	debounceDuration := 1 * time.Second

	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-cm.done:
			return

		case event, ok := <-cm.watcher.Events:
			if !ok {
				return
			}

			if filepath.Base(event.Name) != fileName {
				continue
			}

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(debounceDuration, func() {
					cm.reload()
				})
			}

		case err, ok := <-cm.watcher.Errors:
			if !ok {
				return
			}
			cm.log.Error("config watcher error", "error", err)
		}
	}
}

func (cm *ConfigManager) reload() {
	select {
	case <-cm.done:
		return
	default:
	}

	cm.log.Info("detecting config change, reloading...")

	newConfig, err := Load(cm.path)
	if err != nil {
		cm.log.Error("failed to reload config", "error", err)
		// Record failed reload metric
		if cm.metrics != nil {
			cm.metrics.Config.RecordConfigReload(false)
		}
		return
	}

	cm.config.Store(newConfig)

	select {
	case cm.reloadCh <- struct{}{}:
	default:
	}

	cm.log.Info("config reloaded successfully")
	// Record successful reload metric
	if cm.metrics != nil {
		cm.metrics.Config.RecordConfigReload(true)
	}
}
