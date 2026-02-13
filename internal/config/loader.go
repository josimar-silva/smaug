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
// 2. Applies environment variable substitution to the file content
// 3. Parses the YAML content into the Config struct
// 4. Returns the populated Config or an error with context
//
// Environment variable substitution supports:
// - ${VAR_NAME} - Replaced with the value of VAR_NAME, or empty string if not set
// - ${VAR_NAME:-default} - Replaced with VAR_NAME value, or default if VAR_NAME not set
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

	content := SubstituteEnv(string(data))

	var config Config
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}
