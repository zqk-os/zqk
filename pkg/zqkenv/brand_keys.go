package zqkenv

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/brand"
)

func DefaultBrandKey(suffix string) string {
	suffix = strings.TrimPrefix(suffix, "_")
	return brand.DefaultEnvPrefix + "_" + suffix
}

func AdminBrandKey(suffix string) string {
	suffix = strings.TrimPrefix(suffix, "_")
	return brand.DefaultEnvPrefix + "_ADMIN_" + suffix
}

func DefaultAssignment(suffix, value string) string {
	return DefaultBrandKey(suffix) + "=" + value
}

func AdminAssignment(suffix, value string) string {
	return AdminBrandKey(suffix) + "=" + value
}

func HasDefaultAssignment(entry, suffix string) bool {
	return strings.HasPrefix(entry, DefaultBrandKey(suffix)+"=")
}

func IsProductPrefixed(entry string) bool {
	cur := brand.EnvPrefix() + "_"
	def := brand.DefaultEnvPrefix + "_"
	if strings.HasPrefix(entry, cur) {
		return true
	}
	return cur != def && strings.HasPrefix(entry, def)
}

func Airgap() EnvVar               { return EnvVar{Key: brand.EnvVar("AIRGAP")} }
func AllowDegraded() EnvVar        { return EnvVar{Key: brand.EnvVar("ALLOW_DEGRADED")} }
func BypassHandslapper() EnvVar    { return EnvVar{Key: brand.EnvVar("BYPASS_HANDSLAPPER")} }
func Codegen() EnvVar              { return EnvVar{Key: brand.EnvVar("CODEGEN")} }
func ContextProfile() EnvVar       { return EnvVar{Key: brand.EnvVar("CONTEXT_PROFILE")} }
func DebugOperations() EnvVar      { return EnvVar{Key: brand.EnvVar("DEBUG_OPERATIONS")} }
func DevCodegen() EnvVar           { return EnvVar{Key: brand.EnvVar("DEV_CODEGEN")} }
func DisableLocalOllama() EnvVar   { return EnvVar{Key: brand.EnvVar("DISABLE_LOCAL_OLLAMA")} }
func DistDir() EnvVar              { return EnvVar{Key: brand.EnvVar("DIST_DIR")} }
func ForceLocalOllama() EnvVar     { return EnvVar{Key: brand.EnvVar("FORCE_LOCAL_OLLAMA")} }
func InfoOperations() EnvVar       { return EnvVar{Key: brand.EnvVar("INFO_OPERATIONS")} }
func LogLevel() EnvVar             { return EnvVar{Key: brand.EnvVar("LOG_LEVEL")} }
func NonInteractive() EnvVar       { return EnvVar{Key: brand.EnvVar("NON_INTERACTIVE")} }
func ObserverConcurrency() EnvVar  { return EnvVar{Key: brand.EnvVar("OBSERVER_CONCURRENCY")} }
func ObserverIncludeTests() EnvVar { return EnvVar{Key: brand.EnvVar("OBSERVER_INCLUDE_TESTS")} }
func ObserverSkipIntent() EnvVar   { return EnvVar{Key: brand.EnvVar("OBSERVER_SKIP_INTENT")} }
func Profile() EnvVar              { return EnvVar{Key: brand.EnvVar("PROFILE")} }
func TaskID() EnvVar               { return EnvVar{Key: brand.EnvVar("TASK_ID")} }
func Timeout() EnvVar              { return EnvVar{Key: brand.EnvVar("TIMEOUT")} }
func Env() EnvVar                  { return EnvVar{Key: brand.EnvVar("ENV")} }
