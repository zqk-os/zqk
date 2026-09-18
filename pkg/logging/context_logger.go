package logging

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// GetLoggerFromContext creates an EventLogger based on CLI context
// First tries to extract LoggingContext from the Go context
// Falls back to extracting profile string from context values for backward compatibility
// This integrates with internal/cli/context by reading the profile from context values
func GetLoggerFromContext(ctx context.Context) *EventLogger {
	return NewEventLogger(ctx)
}

// getLoggingContextFromGoContext extracts LoggingContext from Go context
// Falls back to creating one from profile string if LoggingContext is not present
func getLoggingContextFromGoContext(ctx context.Context) *pkgctx.LoggingContext {
	// Try to get LoggingContext directly
	if loggingCtx := pkgctx.GetLoggingContext(ctx); loggingCtx != nil {
		return loggingCtx
	}

	// Fallback: extract profile string and create LoggingContext
	profile := extractProfileStringFromContext(ctx)
	return createLoggingContextFromProfile(profile)
}

// extractProfileStringFromContext extracts the profile string from various context types
// This is a fallback for backward compatibility when LoggingContext is not present
func extractProfileStringFromContext(ctx context.Context) string {
	// Try direct profile value
	if profileVal := ctx.Value("profile"); profileVal != nil {
		if p, ok := profileVal.(string); ok {
			return p
		}
	}

	// Try cli_context wrapper (using reflection)
	if cliCtx := ctx.Value("cli_context"); cliCtx != nil {
		val := reflect.ValueOf(cliCtx)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		if val.Kind() == reflect.Struct {
			profileField := val.FieldByName("Profile")
			if profileField.IsValid() && profileField.Kind() == reflect.String {
				return profileField.String()
			}
		}
	}

	return string(pkgctx.ProfileHuman) // Default
}

// createLoggingContextFromProfile creates a LoggingContext from a profile string
// This maintains backward compatibility with string-based profiles
func createLoggingContextFromProfile(profile string) *pkgctx.LoggingContext {
	// Handle MCP mode detection
	if (profile == emptyValue || profile == string(pkgctx.ProfileHuman)) && isMCPMode() {
		profile = string(pkgctx.ProfileMCP)
	}

	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case string(pkgctx.ProfileMCP):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case string(pkgctx.ProfileSystem):
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case string(pkgctx.ProfileAIAgent):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
		// In MCP mode, suppress debug to stdout
		if isMCPMode() {
			loggingCtx = loggingCtx.WithSuppressDebugToStdout(true)
		}
	case string(pkgctx.ProfileDebug):
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
		// In MCP mode, suppress debug to stdout
		if isMCPMode() {
			loggingCtx = loggingCtx.WithSuppressDebugToStdout(true)
		}
	case string(pkgctx.ProfileHuman), "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
		// In MCP mode, convert to MCP profile
		if isMCPMode() {
			loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
		}
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
		if isMCPMode() {
			loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
		}
	}

	return loggingCtx
}

// isMCPMode detects if we're running in MCP mode by checking environment variables
// MCP mode is detected when ZQK_MCP_ACCOUNT_ID is set (set by MCP server when executing commands)
// IMPORTANT: This should only be true in subprocesses spawned by the MCP server, not in the
// MCP server process itself. The MCP server process should use explicit "mcp" profile, not
// rely on environment variable detection.
func isMCPMode() bool {
	// Check for ZQK_MCP_ACCOUNT_ID which is only set in subprocesses by ExecuteCLICommandViaMCP
	// This ensures MCP mode detection is session-specific and doesn't pollute parent processes
	return zqkenv.MCPAccountID().Get() != emptyValue
}

// loggerCache caches Logger instances by profile string to avoid expensive repeated creation
var loggerCache sync.Map // map[string]Logger

// GetLoggerFromProfile creates a logger from a profile string
// This function uses the global LogRouter if available, which routes to both file and stdio destinations
// Profiles:
//   - "mcp": Writes to stderr (stdout reserved for JSON-RPC), JSON format, Debug level
//   - "system": Writes to stderr (system logs shouldn't pollute stdout), JSON format, Debug level
//   - "ai-agent": Writes to stdout, JSON format, Debug level
//   - "debug": Writes to stdout, Compact format, Debug level
//   - "human", "": Writes to stdout, Text format, Info level (unless in MCP mode, then uses "mcp" profile)
//
// This function maintains backward compatibility with string-based profiles
// Results are cached to avoid expensive repeated creation for the same profile
// If parentCtx is provided, the LoggingContext is combined with the parent context
func GetLoggerFromProfile(profile string, parentCtx ...context.Context) Logger {
	// Create cache key that includes MCP mode since it affects the result
	// MCP mode is checked inside createLoggingContextFromProfile, so we need to include it in the key
	mcpMode := isMCPMode()
	cacheKey := fmt.Sprintf("%s:mcp=%v", profile, mcpMode)

	// Check cache first (only if no parent context provided, as context affects logger behavior)
	if len(parentCtx) == 0 {
		if cached, ok := loggerCache.Load(cacheKey); ok {
			return cached.(Logger)
		}
	}

	// Create LoggingContext from profile string
	loggingCtx := createLoggingContextFromProfile(profile)

	// Combine with parent context if provided
	if len(parentCtx) > 0 && parentCtx[0] != nil {
		// Combine LoggingContext with parent context
		combinedCtx := pkgctx.WithLoggingContext(parentCtx[0], loggingCtx)
		// Extract the combined LoggingContext (which may have been merged with parent's LoggingContext)
		combinedLoggingCtx := getLoggingContextFromGoContext(combinedCtx)
		// Use GetLoggerFromLoggingContext with the combined context (pass parent context for formatter)
		logger := GetLoggerFromLoggingContext(combinedCtx, combinedLoggingCtx)
		// Only cache if no parent context (context-specific loggers shouldn't be cached)
		if len(parentCtx) == 0 {
			loggerCache.Store(cacheKey, logger)
		}
		return logger
	}

	// No parent context - use direct LoggingContext (backward compatible)
	// Use system context as fallback when no parent context is provided
	logger := GetLoggerFromLoggingContext(pkgctx.NewSystemContext(), loggingCtx)

	// Cache the result
	loggerCache.Store(cacheKey, logger)
	return logger
}

// GetLoggerFromLoggingContext creates a logger from a LoggingContext
// This is the new preferred way to create loggers
// ctx: parent context for cancellation and context-aware formatting
func GetLoggerFromLoggingContext(ctx context.Context, loggingCtx *pkgctx.LoggingContext) Logger {
	if loggingCtx == nil {
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileHuman)
	}
	// Determine profile-specific settings
	var formatter Formatter
	var level LogLevel
	var writer io.Writer

	profile := string(loggingCtx.Profile)

	switch loggingCtx.Profile {
	case pkgctx.ProfileMCP:
		// MCP profile: stderr for logs, stdout reserved for JSON-RPC protocol
		formatter = NewJSONFormatter(ctx)
		level = DebugLevel
		writer = os.Stderr
	case pkgctx.ProfileSystem:
		// System profile: stderr for system logs (warnings, errors from internal operations)
		// This prevents system logs from polluting stdout, which is reserved for user output
		formatter = NewJSONFormatter(ctx)
		level = DebugLevel
		writer = os.Stderr
	case pkgctx.ProfileAIAgent:
		formatter = NewJSONFormatter(ctx)
		level = DebugLevel
		// Check if we should suppress debug to stdout
		if loggingCtx.ShouldSuppressDebugToStdout() || isMCPMode() {
			writer = os.Stderr
		} else {
			writer = os.Stdout
		}
	case pkgctx.ProfileDebug:
		formatter = NewCompactFormatter(ctx)
		level = DebugLevel
		// Check if we should suppress debug to stdout
		if loggingCtx.ShouldSuppressDebugToStdout() || isMCPMode() {
			writer = os.Stderr
		} else {
			writer = os.Stdout
		}
	case pkgctx.ProfileHuman:
		formatter = NewTextFormatter(ctx)
		level = ErrorLevel // Human profile should only show errors, suppress warnings and debug
		// Human profile should suppress raw developer logging to stdout
		// All user-facing output is handled by cli.WriteOutput
		writer = io.Discard
	default:
		formatter = NewTextFormatter(ctx)
		level = InfoLevel
		if isMCPMode() {
			writer = os.Stderr
		} else {
			writer = os.Stdout
		}
	}

	// Override level if specified in LoggingContext
	if loggingCtx.Level != nil {
		level = LogLevel(*loggingCtx.Level)
	}

	// Get global router (always non-nil, lazy-initialized if needed)
	router := GetGlobalRouter()

	// CRITICAL: System profile should NOT add stdio destinations
	// System logs are internal operations that should only go to log files, not stdio
	// This prevents system debug logs from polluting stdout/stderr
	// Also skip stdio destinations in MCP mode (subprocesses) - stdout is reserved for JSON-RPC protocol
	// CRITICAL: MCP profile should NOT add stdio destinations when used by the MCP server process
	// The MCP server process should only write to log files to avoid stderr pollution
	// (stderr is reserved for JSON-RPC protocol errors, not info-level logs)
	isMCPMode := zqkenv.MCPAccountID().Get() != emptyValue
	isMCPServerProcess := loggingCtx.Profile == pkgctx.ProfileMCP && !isMCPMode
	if loggingCtx.Profile != pkgctx.ProfileSystem && !isMCPMode && !isMCPServerProcess {
		// Add stdio destination for this profile to the router (if not already added)
		// Use a consistent naming scheme: "stdio_<profile>"
		// Skip in MCP mode because stdout/stderr are reserved for JSON-RPC protocol
		// Skip for MCP server process to prevent stderr pollution (info logs should only go to files)
		// If destination already exists, AddDestination will update it if the new level is more permissive
		destName := fmt.Sprintf("stdio_%s", profile)
		router.AddDestination(destName, writer, level, formatter)
	}

	// Return the router's logger (which routes to file destinations and optionally stdio)
	// Pass LoggingContext to router so it can check SuppressDebugToStdout
	return router.GetLoggerWithLoggingContext(loggingCtx)
}

// GetLoggerFromDecisionContext creates a logger from a LoggingDecisionContext
// This is the most context-aware way to create loggers, using all available contexts
// to determine what to log, when to log, which channel, and which file
func GetLoggerFromDecisionContext(decisionCtx *pkgctx.LoggingDecisionContext, projectRoot string) Logger {
	// Get operation context from decision context (for formatter context propagation)
	// Use system context as fallback
	ctx := pkgctx.NewSystemContext()
	if decisionCtx != nil && decisionCtx.OperationCtx != nil {
		ctx = decisionCtx.OperationCtx
	}

	if decisionCtx == nil {
		// Fallback to default LoggingContext
		return GetLoggerFromLoggingContext(ctx, pkgctx.NewLoggingContext(pkgctx.ProfileSystem))
	}

	// Use LoggingContext from decision context
	loggingCtx := decisionCtx.LoggingCtx
	if loggingCtx == nil {
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileSystem)
	}

	// Get the router (always non-nil, lazy-initialized if needed)
	router := GetGlobalRouter()

	// Create a context-aware router logger that uses LoggingDecisionContext
	return &decisionContextLogger{
		router:      router,
		decisionCtx: decisionCtx,
		loggingCtx:  loggingCtx,
		projectRoot: projectRoot,
	}
}

// GetLogger creates an EventLogger with default settings (no context)
// For context-aware logging, use GetLoggerFromContext() or NewEventLogger() instead
//
// Deprecated: Use NewEventLogger(pkgctx.NewSystemContext()) or GetLoggerFromContext(ctx) instead
func GetLogger() *EventLogger {
	return NewEventLogger(pkgctx.NewSystemContext())
}
