package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads and parses a YAML configuration file from the given path.
// It returns a pointer to the Config struct if successful, or an error if the file
// cannot be read or contains malformed YAML.
//
// The function performs the following steps:
// 1. Reads the file from the filesystem
// 2. Parses the YAML content into the Config struct
// 3. Returns the populated Config or an error with context
//
// Error cases:
// - File does not exist or cannot be read: returns "failed to read config file" error
// - Invalid YAML syntax: returns "failed to parse config file" error
func Load(path string) (*Config, error) {
	// Read the configuration file
	// #nosec G304 -- Config file path is expected to be user-provided and validated by caller
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Parse the YAML content
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}
