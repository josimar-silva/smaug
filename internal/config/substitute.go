package config

import (
	"os"
	"regexp"
)

// SubstituteEnv replaces environment variable references in a string.
//
// Supports two syntaxes:
// - ${VAR_NAME} - Replaced with the value of VAR_NAME, or empty string if not set
// - ${VAR_NAME:-default} - Replaced with VAR_NAME value, or default if VAR_NAME not set
//
// If a variable is set to an empty string, the empty value is used (not the default).
// Defaults are only used when the variable is completely unset.
func SubstituteEnv(value string) string {
	// Pattern to match ${VAR_NAME} or ${VAR_NAME:-default}
	// Captures: variable name and optional default value
	pattern := regexp.MustCompile(`\$\{([^}:]+)(?::-([^}]*))?\}`)

	return pattern.ReplaceAllStringFunc(value, func(match string) string {
		groups := pattern.FindStringSubmatch(match)
		if len(groups) < 2 {
			return match
		}

		varName := groups[1]
		defaultValue := ""
		if len(groups) > 2 {
			defaultValue = groups[2]
		}

		// Check if variable is set (even if empty)
		envValue, exists := os.LookupEnv(varName)
		if exists {
			return envValue
		}

		// Variable not set - use default or empty string
		return defaultValue
	})
}
