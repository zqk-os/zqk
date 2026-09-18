package zqkenv

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/zqk-os/zqk/pkg/brand"
)

// unreachableTestSocketBasename names a socket that is deliberately never created, so tests can
// exercise fail-closed / fallthrough paths on a dial failure.
const unreachableTestSocketBasename = "non-existent-test.sock"

// UnreachableTestSocketPath returns a socket path under [os.TempDir] that no daemon listens on.
// Built from the temp dir and brand name rather than a hardcoded /tmp/zqk-* literal so it honors
// TMPDIR and stays valid on any OS.
func UnreachableTestSocketPath() string {
	return filepath.Join(os.TempDir(), brand.ExecutableName()+"-"+unreachableTestSocketBasename)
}

// EnvSetter matches [testing.T.Setenv] and [testing.B.Setenv] so the same isolation env can be
// applied from tests, benchmarks, and TestMain/init (through [OSEnvSetter]) without each call site
// repeating the key list.
type EnvSetter func(key, value string)

// OSEnvSetter adapts [os.Setenv] to [EnvSetter] for init/TestMain call sites, where no *testing.T
// exists and process env is the only scope available.
func OSEnvSetter(key, value string) { _ = os.Setenv(key, value) }

// ApplyIsolatedStorageEnv points the privileged-writer socket at a path no daemon listens on so
// tests do not dial a developer's live PW. CAS fallthrough is enabled only when [TestRoot] is
// already bound: fallthrough without an isolated TEST_ROOT writes into the live workspace
// (stewardship incident 2026-08-12 — "Test User" accounts landed under .zqk/process/).
//
// Callers must set TEST_ROOT (and clear PROJECT_ROOT) before this helper when they need local CAS
// writes. Prefer [pkg/testkit.PrepareIsolatedTempProject].
// TRACK: BLI-CAS-HAND-DUP-CHECK-001 — remove when: create path refuses fixture IDs into zqk:kernel
// outside TEST_ROOT regardless of fallthrough.
func ApplyIsolatedStorageEnv(set EnvSetter) {
	set(PrivilegedWriterSocket().Name(), UnreachableTestSocketPath())
	if TestRoot().Get() == "" {
		return
	}
	set(TestAllowCASFallthrough().Name(), enabledFlagValue)
}

// enabledFlagValue is the truthy value this project writes for boolean env flags.
const enabledFlagValue = "1"

// DaemonProcess reports whether brand IS_DAEMON=1. Prefer
// [PrivilegedWriterDaemonRole] at contract sites — that is true from argv
// even when Setenv lost the race with storage init.
func DaemonProcess() bool {
	return IsDaemon().Get() == enabledFlagValue
}

// EnvValue is a wrapper around an environment variable value that provides convenient
// fallback and type conversion methods.
type EnvValue struct {
	Key string
	Val string
}

// Get returns an EnvValue for the given environment variable key.
func Get(key string) EnvValue {
	return EnvValue{
		Key: key,
		Val: os.Getenv(key),
	}
}

// OrDefault returns the environment variable value if it is not empty,
// otherwise it returns the provided default value.
func (e EnvValue) OrDefault(def string) string {
	if e.Val != "" {
		return e.Val
	}
	return def
}

// IntOrDefault returns the environment variable value parsed as an integer
// if it is valid, otherwise it returns the provided default value.
func (e EnvValue) IntOrDefault(def int) int {
	if e.Val != "" {
		if parsed, err := strconv.Atoi(e.Val); err == nil {
			return parsed
		}
	}
	return def
}

// BoolOrDefault returns the environment variable value parsed as a boolean
// if it is valid, otherwise it returns the provided default value.
func (e EnvValue) BoolOrDefault(def bool) bool {
	if e.Val != "" {
		if parsed, err := strconv.ParseBool(e.Val); err == nil {
			return parsed
		}
	}
	return def
}
