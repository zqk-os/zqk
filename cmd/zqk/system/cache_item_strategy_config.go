package system

import (
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/logging"
)

// LoadCacheItemStrategiesFromEnv loads cache item strategies from environment variables
// This provides a simple, fast configuration mechanism for diagnostic strategies
//
// Current Implementation (Simple):
// - Environment variable based (fast iteration, easy enable/disable)
// - Programmatic registration (flexible, code-based)
//
// Future Evolution (When Needed):
// - Spec-based configuration (cache_item_strategy objects)
// - YAML configuration files
// - Performance tuning controls (cache size limits, eviction policies, etc.)
// - Granular per-object-type strategies
// - Metrics and alerting integration
//
// The current simple approach is sufficient for stability/reliability work.
// When performance tuning requires more granular controls, we can evolve to
// spec-based configuration without breaking the existing interface.
//
// Environment variables:
//   - ZQK_CACHE_DIAGNOSTIC_OBJECTS: Comma-separated list of object IDs to track (e.g., "REQ-999,CRIT-9091,CRIT-9092")
//   - ZQK_CACHE_DIAGNOSTIC_PREFIXES: Comma-separated list of prefixes to track (e.g., "REQ-,CRIT-")
//   - ZQK_CACHE_DIAGNOSTIC_ENABLED: Set to "true" to enable diagnostic strategies (default: false)
//
// Example usage:
//
//	export ZQK_CACHE_DIAGNOSTIC_ENABLED=true
//	export ZQK_CACHE_DIAGNOSTIC_OBJECTS="REQ-999,CRIT-9091,CRIT-9092"
func LoadCacheItemStrategiesFromEnv() {
	if !config.MaintenanceCacheDiagnosticEnabled().OrDefault(false) {
		return // Diagnostics disabled
	}

	registry := GetGlobalCacheItemStrategyRegistry()

	// Load specific object IDs
	if objectsEnv := config.MaintenanceCacheDiagnosticObjects().OrDefault(""); objectsEnv != "" {
		objectIDs := parseCommaSeparatedList(objectsEnv)
		if len(objectIDs) > 0 {
			strategy := NewDiagnosticCacheItemStrategy("env-diagnostic-objects", objectIDs)
			registry.RegisterStrategy(strategy)
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(logger).Debug("Registered diagnostic cache item strategy from environment").
				String("strategy", strategy.Name()).
				ObjectCount(len(objectIDs)).
				Log()
		}
	}

	// Load prefix patterns
	if prefixesEnv := config.MaintenanceCacheDiagnosticPrefixes().OrDefault(""); prefixesEnv != "" {
		prefixes := parseCommaSeparatedList(prefixesEnv)
		for _, prefix := range prefixes {
			if prefix != "" {
				strategy := NewPrefixBasedCacheItemStrategy("env-diagnostic-prefix-"+prefix, prefix, logging.WarnLevel)
				registry.RegisterStrategy(strategy)
				logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
				logging.Fluent(logger).Debug("Registered prefix-based cache item strategy from environment").
					String("strategy", strategy.Name()).
					String("prefix", prefix).
					Log()
			}
		}
	}
}

// parseCommaSeparatedList parses a comma-separated list and trims whitespace
func parseCommaSeparatedList(s string) []string {
	if s == emptyValue {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != emptyValue {
			result = append(result, trimmed)
		}
	}
	return result
}

// RegisterDefaultDiagnosticStrategies registers default diagnostic strategies
// This can be called during system initialization to track known problematic objects
// The objects can be configured via environment variables or removed if not needed
func RegisterDefaultDiagnosticStrategies() {
	// Load from environment first (allows override)
	LoadCacheItemStrategiesFromEnv()

	// If no strategies were loaded from env, optionally register defaults here
	// For now, we rely on environment configuration to avoid hardcoding
}
