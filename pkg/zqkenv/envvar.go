package zqkenv

import (
	"fmt"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
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
// key is unset, it falls back to the default ZQK_* alias — except community
// PROJECT_ROOT / TEST_ROOT, which must not inherit a studio IDE's ZQK_PROJECT_ROOT
// (that makes `cd /tmp && zcom system init` hit the studio kernel).
// TRACK: TDE-1789681135032251000-e52ad7a7
func (e EnvVar) Get() string {
	if val := os.Getenv(e.Key); val != "" {
		return val
	}
	if IsCommunityEdition && communityMustNotInheritStudioRoot(e.Key) {
		return ""
	}
	if pfx := brand.EnvPrefix(); pfx != brand.DefaultEnvPrefix && strings.HasPrefix(e.Key, pfx+"_") {
		fallbackKey := brand.DefaultEnvPrefix + "_" + strings.TrimPrefix(e.Key, pfx+"_")
		if val := os.Getenv(fallbackKey); val != "" {
			return val
		}
	}
	return ""
}

func communityMustNotInheritStudioRoot(key string) bool {
	pfx := brand.EnvPrefix()
	if pfx == brand.DefaultEnvPrefix || !strings.HasPrefix(key, pfx+"_") {
		return false
	}
	suf := strings.TrimPrefix(key, pfx+"_")
	return suf == "PROJECT_ROOT" || suf == "TEST_ROOT"
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

// Required panics if the environment variable is not present.
func (e EnvVar) Required() string {
	val := e.Get()
	if val == "" {
		panic(fmt.Sprintf("⚡️ FATAL: Missing required environment variable: %s", e.Key))
	}
	return val
}
