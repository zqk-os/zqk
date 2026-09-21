package cli

import (
	"maps"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
)

// commandTimeoutsConfig is the on-disk format for command timeout overrides.
type commandTimeoutsConfig struct {
	DefaultMax string               `yaml:"default_max"`
	Rules      []commandTimeoutRule `yaml:"rules"`
}

type commandTimeoutRule struct {
	Pattern string `yaml:"pattern"`
	Timeout string `yaml:"timeout"`

	// ChildMaxTimeoutExempt skips the childMaxTimeout cap (2m) for this command when
	// the parent process is not zqk. Use for commands that are long-running by design
	// (e.g. scan-tests --all).
	ChildMaxTimeoutExempt bool `yaml:"child_max_timeout_exempt,omitempty"`

	// ChildMaxTimeoutExemptIfExplicit skips the childMaxTimeout cap only when the user
	// explicitly passed --timeout on the command line. Use for commands where a long run
	// is only expected when the caller opted in (e.g. system check).
	ChildMaxTimeoutExemptIfExplicit bool `yaml:"child_max_timeout_exempt_if_explicit,omitempty"`

	// IdleShutdownDuration overrides the default idle watchdog window for this command.
	// Parsed as a Go duration string (e.g. "2m", "5m"). Empty means use the default (10s).
	IdleShutdownDuration string `yaml:"idle_shutdown_duration,omitempty"`
}

// getTimeoutForCommand gets the timeout for a command based on metrics
// args are passed for dynamic calculation (e.g., system check needs object count estimation)
func (h *TimeoutHook) getTimeoutForCommand(normalizedCmd string, args []string) time.Duration {
	var commandTimeoutsCopy map[string]time.Duration
	var maxTimeout time.Duration
	var metricsStore MetricsStore
	_ = concurrency.RunInRLockWithLogger(
		&h.mu, LockNameTimeoutHookGetTimeoutCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			commandTimeoutsCopy = make(map[string]time.Duration, len(h.commandTimeouts))
			if h.commandTimeouts != nil {
				maps.Copy(commandTimeoutsCopy, h.commandTimeouts)
			}
			maxTimeout = h.maxTimeout
			metricsStore = h.metricsStore
			return nil
		},
	)

	// Check for command-specific timeout first (highest priority) using snapshot from lock.
	if timeout, exists := commandTimeoutsCopy[normalizedCmd]; exists {
		return timeout
	}

	// Pattern-based timeouts from config/command_timeouts.yaml (or .zqk/config override)
	if timeout := h.getTimeoutFromCommandTimeoutsConfig(normalizedCmd, maxTimeout); timeout > 0 {
		return timeout
	}

	// MCP long-lived processes: timeout from .zqk/mcp/config.yaml (mcp_server.idle_timeout),
	// not command_timeouts.yaml. Includes mcp daemon/proxy (TCP ship path), not only mcp serve.
	if strings.Contains(normalizedCmd, "mcp serve") ||
		strings.Contains(normalizedCmd, "mcp daemon") ||
		strings.Contains(normalizedCmd, "mcp proxy") ||
		strings.Contains(normalizedCmd, "mcp supervise") ||
		strings.Contains(normalizedCmd, "mcp ide-adapter") ||
		strings.Contains(normalizedCmd, "mcp cursor-adapter") {
		mcpTimeout, isDisabled := h.getMCPIdleTimeout()
		if isDisabled {
			// Timeout is disabled (idle_timeout: "0") - return a very large duration
			// that will be treated as infinite by wrapCommandWithResettableTimeout
			h.logger.LogDebug("MCP idle timeout is disabled (idle_timeout: \"0\")",
				logging.String("command", normalizedCmd))
			config := h.getTimeoutConfig()
			return config.InfiniteTimeout
		}
		if mcpTimeout > 0 {
			// Log that we're using MCP config timeout (for debugging)
			h.logger.LogDebug("Using MCP config idle timeout",
				logging.String("command", normalizedCmd),
				logging.String("timeout", mcpTimeout.String()))
			return mcpTimeout
		}
		// If MCP config read failed or not set, continue to fallback
	}

	if metricsStore == nil {
		return maxTimeout
	}

	metrics, err := metricsStore.GetCommandMetrics(normalizedCmd)
	if err != nil || metrics == nil {
		return h.maxTimeout
	}

	// Calculate timeout based on baseline + margin
	// Use baseline * 3 as timeout (allows for variance, but prevents runaway timeouts)
	// CRITICAL: Only use baseline from successful runs (timed-out runs don't affect baseline)
	// Cap timeout growth to prevent runaway scenarios
	if metrics.BaselineDuration > 0 {
		// Use baseline * 3 for more generous timeout (was * 2)
		// This prevents premature timeouts while still having a reasonable limit
		timeout := metrics.BaselineDuration * 3
		if timeout > maxTimeout {
			return maxTimeout
		}
		// For "system check" commands, calculate timeout dynamically based on estimated object count
		// This matches the calculation in async_check.go for consistency
		if strings.Contains(normalizedCmd, "system check") {
			// Estimate object count from command args
			estimatedObjectCount := h.estimateObjectCountForSystemCheck(args)

			// Use same calculation as async_check.go with config values
			config := h.getTimeoutConfig()
			estimatedTime := time.Duration(estimatedObjectCount) * config.SystemCheckTimeoutConfig.BaseTimePerObject / time.Duration(config.SystemCheckTimeoutConfig.WorkerCount)
			// Add safety margin + overhead for enqueueing and result collection
			dynamicTimeout := time.Duration(float64(estimatedTime)*config.SystemCheckTimeoutConfig.SafetyMargin) + config.SystemCheckTimeoutConfig.Overhead

			// Use the larger of: baseline-based timeout or dynamic calculation
			// This ensures we don't timeout prematurely for bulk operations
			if dynamicTimeout > timeout {
				timeout = dynamicTimeout
			}

			// CRITICAL: If --auto-fix is used, enforce minimum 2-minute timeout
			// Metrics show 100% error rate for --auto-fix with short timeouts (30s, 60s)
			// Auto-fix operations require time to apply fixes, so short timeouts always fail
			if strings.Contains(normalizedCmd, "--auto-fix") {
				minAutoFixTimeout := 2 * time.Minute
				if timeout < minAutoFixTimeout {
					timeout = minAutoFixTimeout
				}
			}

			// Apply reasonable bounds from config
			if timeout < config.SystemCheckTimeoutConfig.MinTimeout {
				timeout = config.SystemCheckTimeoutConfig.MinTimeout
			}
			// Floor so a low baseline (e.g. 93s → 3x = 4m40s) doesn't kill long-running checks
			if minReasonable := time.Duration(config.SystemCheckTimeoutConfig.MinReasonableMinutes) * time.Minute; minReasonable > 0 && timeout < minReasonable {
				timeout = minReasonable
			}
			if timeout > config.SystemCheckTimeoutConfig.MaxTimeout {
				timeout = config.SystemCheckTimeoutConfig.MaxTimeout
			}
			if timeout > maxTimeout {
				timeout = maxTimeout
			}
			return timeout
		}

		// For non-system-check commands, use standard timeout calculation
		// Cap maximum timeout growth to prevent runaway scenarios
		// CRITICAL: For "mcp serve" commands, skip the 5-minute cap if MCP config read failed
		// This ensures server commands can use longer timeouts when needed
		// Only apply cap to non-server commands
		if !strings.Contains(normalizedCmd, "mcp serve") {
			// Even with baseline * 3, don't let it grow beyond reasonable limit unless maxTimeout is higher
			config := h.getTimeoutConfig()
			maxReasonableTimeout := config.DefaultTimeoutBounds.MaxReasonableTimeout
			if h.maxTimeout < maxReasonableTimeout {
				maxReasonableTimeout = h.maxTimeout
			}
			if timeout > maxReasonableTimeout {
				return maxReasonableTimeout
			}
		}

		// Minimum timeout for all commands (safety net)
		config := h.getTimeoutConfig()
		minTimeout := config.DefaultTimeoutBounds.MinTimeout
		if timeout < minTimeout {
			return minTimeout
		}
		return timeout
	}

	// No baseline available - for "system check", use dynamic calculation
	if strings.Contains(normalizedCmd, "system check") {
		config := h.getTimeoutConfig()
		estimatedObjectCount := h.estimateObjectCountForSystemCheck(args)
		estimatedTime := time.Duration(estimatedObjectCount) * config.SystemCheckTimeoutConfig.BaseTimePerObject / time.Duration(config.SystemCheckTimeoutConfig.WorkerCount)
		timeout := time.Duration(float64(estimatedTime)*config.SystemCheckTimeoutConfig.SafetyMargin) + config.SystemCheckTimeoutConfig.Overhead

		if timeout < config.SystemCheckTimeoutConfig.MinTimeout {
			timeout = config.SystemCheckTimeoutConfig.MinTimeout
		}
		if minReasonable := time.Duration(config.SystemCheckTimeoutConfig.MinReasonableMinutes) * time.Minute; minReasonable > 0 && timeout < minReasonable {
			timeout = minReasonable
		}
		if timeout > config.SystemCheckTimeoutConfig.MaxTimeout {
			timeout = config.SystemCheckTimeoutConfig.MaxTimeout
		}
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
		return timeout
	}

	return maxTimeout
}

// loadCommandTimeoutsConfig reads the effective command_timeouts.yaml: project-level override
// (.zqk/config/) takes precedence over repo default (config/). Returns nil if neither is found.
// commandTimeouts is keyed by project root. Stamp is the timeouts YAML pair.
var commandTimeouts stampmemo.Table[*commandTimeoutsConfig]

func (h *TimeoutHook) loadCommandTimeoutsConfig() *commandTimeoutsConfig {
	projectRoot := h.findProjectRoot()
	if projectRoot == emptyValue {
		return nil
	}
	files := []string{
		filepath.Join(projectRoot, paths.ProjectDataDir, paths.ConfigDir, paths.CommandTimeoutsConfigFile),
		filepath.Join(projectRoot, "config", paths.CommandTimeoutsConfigFile),
	}
	cfg, _ := commandTimeouts.Load(projectRoot, stampmemo.OfAll(files...), func() (*commandTimeoutsConfig, error) {
		for _, base := range files {
			data, err := fileutil.ReadFile(base)
			if err != nil {
				continue
			}
			var parsed commandTimeoutsConfig
			if yaml.Unmarshal(data, &parsed) != nil {
				continue
			}
			return &parsed, nil
		}
		return nil, nil
	})
	return cfg
}

// findMatchingRule returns the first rule whose pattern is a substring of normalizedCmd,
// regardless of whether Timeout is set. Returns nil if no rule matches.
func findMatchingRule(normalizedCmd string, cfg *commandTimeoutsConfig) *commandTimeoutRule {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		if rule.Pattern != emptyValue && strings.Contains(normalizedCmd, rule.Pattern) {
			return rule
		}
	}
	return nil
}

// getTimeoutFromCommandTimeoutsConfig returns a timeout if the normalized command matches any rule
// in config/command_timeouts.yaml or .zqk/config/command_timeouts.yaml. First match wins. Returns 0 if no match.
func (h *TimeoutHook) getTimeoutFromCommandTimeoutsConfig(normalizedCmd string, maxTimeout time.Duration) time.Duration {
	cfg := h.loadCommandTimeoutsConfig()
	if cfg == nil {
		return 0
	}
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		if rule.Pattern == emptyValue || rule.Timeout == emptyValue {
			continue
		}
		if !strings.Contains(normalizedCmd, rule.Pattern) {
			continue
		}
		d, err := time.ParseDuration(rule.Timeout)
		if err != nil {
			continue
		}
		if d > maxTimeout {
			d = maxTimeout
		}
		return d
	}
	return 0
}

// isChildMaxTimeoutExempt reports whether the command should bypass the childMaxTimeout cap
// (applied when the parent process is not zqk). The result depends on:
//   - ChildMaxTimeoutExempt: always exempt (e.g. scan-tests --all)
//   - ChildMaxTimeoutExemptIfExplicit: exempt only when the caller passed --timeout explicitly
func (h *TimeoutHook) isChildMaxTimeoutExempt(normalizedCmd string, timeoutExplicitlySet bool) bool {
	cfg := h.loadCommandTimeoutsConfig()
	rule := findMatchingRule(normalizedCmd, cfg)
	if rule == nil {
		return false
	}
	return rule.ChildMaxTimeoutExempt || (rule.ChildMaxTimeoutExemptIfExplicit && timeoutExplicitlySet)
}

// getIdleShutdownDuration returns the idle watchdog window for the command, falling back to
// defaultDuration when no rule matches or no IdleShutdownDuration is configured.
func (h *TimeoutHook) getIdleShutdownDuration(normalizedCmd string, defaultDuration time.Duration) time.Duration {
	cfg := h.loadCommandTimeoutsConfig()
	rule := findMatchingRule(normalizedCmd, cfg)
	if rule == nil || rule.IdleShutdownDuration == emptyValue {
		return defaultDuration
	}
	d, err := time.ParseDuration(rule.IdleShutdownDuration)
	if err != nil {
		return defaultDuration
	}
	return d
}

// findProjectRoot walks up from cwd until it finds a directory containing .zqk; returns "" if not found.
func (h *TimeoutHook) findProjectRoot() string {
	wd, err := fileutil.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, paths.ProjectDataDir)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// getMCPIdleTimeout reads the MCP server config and returns the configured idle timeout
// Returns (0, false) if config not found or timeout not configured
// Returns (0, true) if timeout is explicitly disabled (idle_timeout: "0")
// Returns (duration, false) if timeout is configured with a duration
// This avoids circular dependencies by reading the config file directly
func (h *TimeoutHook) getMCPIdleTimeout() (time.Duration, bool) {
	// Try to find project root by looking for .zqk directory
	// Start from current working directory and walk up
	wd, err := fileutil.Getwd()
	if err != nil {
		return 0, false
	}

	// Walk up directory tree to find .zqk
	dir := wd
	for {
		configPath := filepath.Join(dir, paths.ProjectDataDir, paths.MCPDir, paths.MCPConfigFile)
		if _, err := fileutil.Stat(configPath); err == nil {
			// Found config file, try to read it
			data, err := fileutil.ReadFile(configPath)
			if err != nil {
				return 0, false
			}

			// Parse just the IdleTimeout field (minimal struct to avoid importing mcp package)
			var config struct {
				MCPServer struct {
					IdleTimeout string `yaml:"idle_timeout"`
				} `yaml:"mcp_server"`
			}

			if err := yaml.Unmarshal(data, &config); err != nil {
				return 0, false
			}

			// Check if timeout is explicitly disabled
			if config.MCPServer.IdleTimeout == "0" {
				return 0, true // Explicitly disabled
			}

			// Empty string means not configured (use default)
			if config.MCPServer.IdleTimeout == emptyValue {
				return 0, false // Not configured, use fallback
			}

			// Parse duration (e.g., "1h30m", "5m", etc.)
			duration, err := time.ParseDuration(config.MCPServer.IdleTimeout)
			if err != nil {
				return 0, false
			}

			return duration, false
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, stop searching
			break
		}
		dir = parent
	}

	return 0, false
}

// estimateObjectCountForSystemCheck estimates the number of objects to process
// based on command arguments. This is used for dynamic timeout calculation.
func (h *TimeoutHook) estimateObjectCountForSystemCheck(args []string) int {
	// Count specific IDs provided (e.g., "zqk system check BLI-001 BLI-002")
	// IDs typically match pattern: [A-Z]+-\d+
	idCount := 0
	for _, arg := range args {
		// Skip flags
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			continue
		}
		// Check if it looks like an ID (e.g., BLI-001, AUD-123)
		if len(arg) > 4 && arg[3] == '-' {
			// Check if it's a valid ID pattern (letters, dash, digits)
			hasLetters := false
			hasDigits := false
			for i, r := range arg {
				if i < 3 && (r >= 'A' && r <= 'Z') {
					hasLetters = true
				}
				if i > 3 && r >= '0' && r <= '9' {
					hasDigits = true
				}
			}
			if hasLetters && hasDigits {
				idCount++
			}
		}
	}

	// If specific IDs provided, use that count
	if idCount > 0 {
		return idCount
	}

	// Check if a specific kind is provided (e.g., "zqk system check backlog_item")
	// Kinds are typically lowercase with underscores
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") || strings.HasPrefix(arg, "-") {
			continue
		}
		// If it's not "all" and doesn't look like an ID, assume it's a kind
		if arg != "all" && !strings.Contains(arg, "-") && strings.Contains(arg, "_") {
			// Single kind - use conservative estimate (e.g., 1000 objects per kind)
			return 1000
		}
	}

	// No specific IDs or kind - assume checking all objects
	// Use a conservative estimate based on typical project size
	// This is a fallback - the actual count will be discovered during execution
	return 5000
}

// getBaselineDuration gets the baseline duration for a command (for logging)
func (h *TimeoutHook) getBaselineDuration(normalizedCmd string) time.Duration {
	var d time.Duration
	_ = concurrency.RunInRLockWithLogger(
		&h.mu, LockNameTimeoutHookBaselineDuration, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if h.metricsStore == nil {
				return nil
			}
			metrics, err := h.metricsStore.GetCommandMetrics(normalizedCmd)
			if err != nil || metrics == nil {
				return nil
			}
			d = metrics.BaselineDuration
			return nil
		},
	)
	return d
}
