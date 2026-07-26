package flags

import (
	"os"
	"strconv"
	"strings"
)

// BoolEnvOrDefault returns true if the environment variable is set to
// "true", "1", "yes", or "on" (case-insensitive); otherwise it returns
// defaultValue.
func BoolEnvOrDefault(envVar string, defaultValue bool) bool {
	if v := os.Getenv(envVar); v != "" {
		switch strings.ToLower(v) {
		case "true", "1", "yes", "on":
			return true
		default:
			return false
		}
	}
	return defaultValue
}

// EnvOrDefault returns the environment variable value if set and non-empty,
// otherwise the default.
func EnvOrDefault(envVar, defaultValue string) string {
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	return defaultValue
}

// IntEnvOrDefault returns the environment variable value parsed as an int if
// set and valid (>= 0), otherwise the default.
func IntEnvOrDefault(envVar string, defaultValue int) int {
	if v := os.Getenv(envVar); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d >= 0 {
			return d
		}
	}
	return defaultValue
}
