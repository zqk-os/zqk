package zqkenv

import (
	"fmt"
	"os"
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
func (e EnvVar) Get() string {
	return os.Getenv(e.Key)
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
	val := os.Getenv(e.Key)
	if val == "" {
		return def
	}
	return val
}

// Required panics if the environment variable is not present.
func (e EnvVar) Required() string {
	val := os.Getenv(e.Key)
	if val == "" {
		panic(fmt.Sprintf("⚡️ FATAL: Missing required environment variable: %s", e.Key))
	}
	return val
}
