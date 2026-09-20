package zqkenv

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

// EnvVar represents a strongly typed environment variable binding.
type EnvVar struct {
	Key string
}

// Name returns the underlying environment variable string key.
func (e EnvVar) Name() string {
	return e.Key
}

// String returns the underlying environment variable string key, satisfying fmt.Stringer.
func (e EnvVar) String() string {
	return e.Key
}

// Get fetches the environment variable value from the OS.
// When running under a non-default brand prefix (e.g. ZCOM) and the brand-specific
// key is unset, it falls back to the default ZQK_* alias.
func (e EnvVar) Get() string {
	if val := os.Getenv(e.Key); val != "" {
		return val
	}
	if pfx := brand.EnvPrefix(); pfx != brand.DefaultEnvPrefix && strings.HasPrefix(e.Key, pfx+"_") {
		fallbackKey := brand.DefaultEnvPrefix + "_" + strings.TrimPrefix(e.Key, pfx+"_")
		if val := os.Getenv(fallbackKey); val != "" {
			return val
		}
	}
	return ""
}

// Set sets the environment variable value in the OS.
func (e EnvVar) Set(val string) error {
	return os.Setenv(e.Key, val)
}

// Unset clears the environment variable value in the OS.
func (e EnvVar) Unset() error {
	return os.Unsetenv(e.Key)
}

// OrDefault returns the environment variable value if present, otherwise returns def.
func (e EnvVar) OrDefault(def string) string {
	val := e.Get()
	if val == "" {
		return def
	}
	return val
}

// IntOrDefault parses the environment variable as an integer, otherwise returns def.
func (e EnvVar) IntOrDefault(def int) int {
	if val := e.Get(); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return def
}

// BoolOrDefault parses the environment variable as a boolean, otherwise returns def.
func (e EnvVar) BoolOrDefault(def bool) bool {
	if val := e.Get(); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return def
}

// Required panics if the environment variable is not present.
func (e EnvVar) Required() string {
	val := e.Get()
	if val == "" {
		panic(fmt.Sprintf("⚡️ FATAL: Missing required environment variable: %s", e.Key))
	}
	return val
}
