package context

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/paths"
)

// LoggingDecisionContext makes context-aware decisions about logging
// It combines SecurityContext, LoggingContext, MCPServerContext, and operation context
// to determine: what to log, when to log, which channel, and which file
type LoggingDecisionContext struct {
	// Source contexts
	LoggingCtx   *LoggingContext
	SecurityCtx  *SecurityContext
	MCPCtx       *MCPServerContext
	OperationCtx context.Context

	// Decision parameters
	Component    string // Component name (e.g., "mcp_server", "storage", "validation")
	Operation    string // Operation name (e.g., "create_object", "validate_reference")
	ClientID     string // Client ID for client-specific logging
	TraceEnabled bool   // Whether trace logging is enabled
	Verbose      bool   // Whether verbose logging is enabled

	// Chain support
	precedence int
	depth      int
}

// NewLoggingDecisionContext creates a new LoggingDecisionContext
func NewLoggingDecisionContext() *LoggingDecisionContext {
	return &LoggingDecisionContext{
		LoggingCtx:   NewLoggingContext(ProfileSystem),
		SecurityCtx:  NewSystemSecurityContext(),
		MCPCtx:       GetMCPServerContext(),
		OperationCtx: NewSystemContext(),
		Component:    "system",
		precedence:   PrecedenceDefault,
		depth:        DepthRoot,
	}
}

// WithLoggingContext sets the LoggingContext
func (ldc *LoggingDecisionContext) WithLoggingContext(loggingCtx *LoggingContext) *LoggingDecisionContext {
	ldc.LoggingCtx = loggingCtx
	return ldc
}

// WithSecurityContext sets the SecurityContext
func (ldc *LoggingDecisionContext) WithSecurityContext(securityCtx *SecurityContext) *LoggingDecisionContext {
	ldc.SecurityCtx = securityCtx
	return ldc
}

// WithComponent sets the component name
func (ldc *LoggingDecisionContext) WithComponent(component string) *LoggingDecisionContext {
	ldc.Component = component
	return ldc
}

// WithOperation sets the operation name
func (ldc *LoggingDecisionContext) WithOperation(operation string) *LoggingDecisionContext {
	ldc.Operation = operation
	return ldc
}

// WithClientID sets the client ID for client-specific logging
func (ldc *LoggingDecisionContext) WithClientID(clientID string) *LoggingDecisionContext {
	ldc.ClientID = clientID
	return ldc
}

// WithTraceEnabled sets whether trace logging is enabled
func (ldc *LoggingDecisionContext) WithTraceEnabled(enabled bool) *LoggingDecisionContext {
	ldc.TraceEnabled = enabled
	return ldc
}

// WithVerbose sets whether verbose logging is enabled
func (ldc *LoggingDecisionContext) WithVerbose(verbose bool) *LoggingDecisionContext {
	ldc.Verbose = verbose
	return ldc
}

// WithOperationContext sets the operation context
func (ldc *LoggingDecisionContext) WithOperationContext(ctx context.Context) *LoggingDecisionContext {
	ldc.OperationCtx = ctx
	return ldc
}

// ShouldLog determines if a log entry should be written based on context
// Returns true if the log should be written, false if it should be filtered
func (ldc *LoggingDecisionContext) ShouldLog(level int, component, operation string) bool {
	// Always log errors and fatal
	if level >= 3 { // ErrorLevel or FatalLevel
		return true
	}

	// Check if component matches
	if ldc.Component != emptyContextValue && component != emptyContextValue && ldc.Component != component {
		// Component filter is set and doesn't match - check if verbose is enabled
		if !ldc.Verbose {
			return false
		}
	}

	// Check if operation matches
	if ldc.Operation != emptyContextValue && operation != emptyContextValue && ldc.Operation != operation {
		// Operation filter is set and doesn't match - check if verbose is enabled
		if !ldc.Verbose {
			return false
		}
	}

	// Check trace enabled for debug logs
	if level == 0 && !ldc.TraceEnabled { // DebugLevel
		return false
	}

	// Check security context for sensitive operations
	if ldc.SecurityCtx != nil {
		// Check if this is a sensitive operation that requires specific permissions
		// This is a placeholder - actual implementation would check permissions
	}

	return true
}

// GetLogLevel determines the effective log level based on context
func (ldc *LoggingDecisionContext) GetLogLevel() int {
	if ldc.LoggingCtx != nil && ldc.LoggingCtx.Level != nil {
		return *ldc.LoggingCtx.Level
	}

	// Default based on profile
	if ldc.LoggingCtx != nil {
		switch ldc.LoggingCtx.Profile {
		case ProfileMCP, ProfileSystem, ProfileAIAgent, ProfileDebug:
			return 0 // DebugLevel
		case ProfileHuman:
			return 3 // ErrorLevel
		default:
			return 1 // InfoLevel
		}
	}

	return 1 // Default to InfoLevel
}

// GetDestinations determines which destinations should receive this log entry
// Returns a map of destination names to their configurations
func (ldc *LoggingDecisionContext) GetDestinations(level int, projectRoot string) map[string]*LogDestination {
	destinations := make(map[string]*LogDestination)

	// Component-scoped loggers (e.g. validation, fix_executor) write only to
	// .zqk/logs/components/<component>-events.json so the same lines are not
	// duplicated into log-events.json. Default/root context uses Component
	// "system" or empty → aggregate log-events.json only.
	component := strings.TrimSpace(ldc.Component)
	if component != emptyContextValue && component != "system" {
		if componentDest := ldc.getComponentFileDestination(level, projectRoot); componentDest != nil {
			destinations["component_"+component] = componentDest
		}
	} else {
		if fileDest := ldc.getFileDestination(level, projectRoot); fileDest != nil {
			destinations["file"] = fileDest
		}
	}

	// Add client-specific file if client ID is set
	if ldc.ClientID != emptyContextValue {
		clientDest := ldc.getClientFileDestination(level, projectRoot)
		if clientDest != nil {
			destinations["client_"+ldc.ClientID] = clientDest
		}
	}

	// Add stdio destinations based on context
	stdioDests := ldc.getStdioDestinations(level)
	for name, dest := range stdioDests {
		destinations[name] = dest
	}

	// Add MCP channel if in MCP context
	if ldc.MCPCtx != nil && ldc.MCPCtx.IsServing() {
		mcpDest := ldc.getMCPChannelDestination(level)
		if mcpDest != nil {
			destinations["mcp_channel"] = mcpDest
		}
	}

	return destinations
}

// getFileDestination returns the main file destination
func (ldc *LoggingDecisionContext) getFileDestination(level int, projectRoot string) *LogDestination {
	if projectRoot == emptyContextValue {
		return nil
	}

	// Determine file path based on formatter
	var logFilePath string
	if ldc.LoggingCtx != nil {
		// Use profile to determine file path
		switch ldc.LoggingCtx.Profile {
		case ProfileMCP:
			// JSON structured logs go to separate file
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir, paths.MCPTraceLogPrefix+"json")
		case ProfileSystem:
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.LogsDir, "log-events.json")
		default:
			logFilePath = filepath.Join(paths.ProjectDataDir, paths.LogsDir, "log-events.json")
		}
	} else {
		logFilePath = filepath.Join(paths.ProjectDataDir, paths.LogsDir, "log-events.json")
	}

	// Resolve relative paths
	if !filepath.IsAbs(logFilePath) {
		logFilePath = filepath.Join(projectRoot, logFilePath)
	}

	return &LogDestination{
		Name:      "file",
		FilePath:  logFilePath,
		Level:     level,
		Formatter: "json", // Default to JSON for file destinations
	}
}

// getComponentFileDestination returns a component-specific file destination
func (ldc *LoggingDecisionContext) getComponentFileDestination(level int, projectRoot string) *LogDestination {
	if projectRoot == emptyContextValue || ldc.Component == emptyContextValue {
		return nil
	}

	// Sanitize component name for use in file path
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, ldc.Component)

	logFilePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.LogsDir, paths.ComponentsLogDir, sanitized+"-events.json")
	return &LogDestination{
		Name:      "component_" + ldc.Component,
		FilePath:  logFilePath,
		Level:     level,
		Formatter: "json",
	}
}

// getClientFileDestination returns a client-specific file destination
func (ldc *LoggingDecisionContext) getClientFileDestination(level int, projectRoot string) *LogDestination {
	if projectRoot == emptyContextValue || ldc.ClientID == emptyContextValue {
		return nil
	}

	// Sanitize client ID for use in file path
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, ldc.ClientID)

	logFilePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.MCPDir, paths.MCPLogsDir, paths.MCPTraceLogPrefix+sanitized+".log")
	return &LogDestination{
		Name:      "client_" + ldc.ClientID,
		FilePath:  logFilePath,
		Level:     level,
		Formatter: "text", // Text format for client-specific trace files
	}
}

// getStdioDestinations returns stdio destinations based on context
func (ldc *LoggingDecisionContext) getStdioDestinations(level int) map[string]*LogDestination {
	destinations := make(map[string]*LogDestination)

	// Don't add stdio destinations if MCP server is serving (stdout reserved for JSON-RPC)
	if ldc.MCPCtx != nil && ldc.MCPCtx.IsServing() {
		return destinations
	}

	// Don't add stdio destinations for system profile
	if ldc.LoggingCtx != nil && ldc.LoggingCtx.Profile == ProfileSystem {
		return destinations
	}

	// Determine writer and formatter based on profile
	var writer string
	var formatter string
	if ldc.LoggingCtx != nil {
		switch ldc.LoggingCtx.Profile {
		case ProfileMCP:
			writer = "stderr"
			formatter = "json"
		case ProfileAIAgent:
			writer = "stdout"
			formatter = "json"
		case ProfileDebug:
			writer = "stdout"
			formatter = "compact"
		case ProfileHuman:
			writer = "stdout"
			formatter = "text"
		default:
			writer = "stdout"
			formatter = "text"
		}
	} else {
		writer = "stdout"
		formatter = "text"
	}

	// Check if debug should be suppressed to stdout
	suppressDebug := false
	if ldc.LoggingCtx != nil {
		suppressDebug = ldc.LoggingCtx.ShouldSuppressDebugToStdout()
	}

	// Only add stdio destination if level is appropriate
	if level >= ldc.GetLogLevel() {
		// For debug level, check suppression
		if level == 0 && suppressDebug && writer == "stdout" {
			// Redirect to stderr instead
			writer = "stderr"
		}
		destinations["stdio"] = &LogDestination{
			Name:      "stdio",
			Writer:    writer,
			Level:     level,
			Formatter: formatter,
		}
	}

	return destinations
}

// getMCPChannelDestination returns the MCP channel destination
func (ldc *LoggingDecisionContext) getMCPChannelDestination(level int) *LogDestination {
	// MCP channel is only for info level and above
	if level < 1 { // Below InfoLevel
		return nil
	}

	return &LogDestination{
		Name:      "mcp_channel",
		Channel:   "notifications/logMessage",
		Level:     level,
		Formatter: "json",
	}
}

// LogDestination represents a logging destination configuration
type LogDestination struct {
	Name          string         // Destination name (e.g., "file", "stdio", "mcp_channel")
	FilePath      string         // File path (for file destinations)
	Writer        string         // Writer type (stdout, stderr) (for stdio destinations)
	Channel       string         // Channel name (for MCP channel destinations)
	Level         int            // Minimum log level for this destination
	Formatter     string         // Formatter type (json, text, compact)
	RollingPolicy *RollingPolicy // Rolling policy for file destinations (nil = use default)
}

// RollingPolicy defines how log files should be rotated
// This is a forward declaration - the actual type is in pkg/logging
// We use a simple struct here to avoid circular dependencies
type RollingPolicy struct {
	Strategy    string `yaml:"strategy"`     // "size", "time", "size_and_time", "none"
	MaxSize     int64  `yaml:"max_size"`     // Maximum file size in bytes
	MaxFiles    int    `yaml:"max_files"`    // Maximum number of rotated files
	MaxAge      string `yaml:"max_age"`      // Maximum age of log files
	RotateEvery string `yaml:"rotate_every"` // Rotate every N time units
}

// GetPrecedence returns the precedence level for this context
func (ldc *LoggingDecisionContext) GetPrecedence() int {
	return ldc.precedence
}

// SetPrecedence sets the precedence level for this context
func (ldc *LoggingDecisionContext) SetPrecedence(precedence int) {
	ldc.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (ldc *LoggingDecisionContext) GetDepth() int {
	return ldc.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (ldc *LoggingDecisionContext) SetDepth(depth int) {
	ldc.depth = depth
}

// Validate performs validation on this context node
func (ldc *LoggingDecisionContext) Validate() []ValidationError {
	var errors []ValidationError
	if ldc.LoggingCtx == nil {
		errors = append(errors, ValidationError{
			Field:   "LoggingCtx",
			Message: "LoggingContext cannot be nil",
			Context: "LoggingDecisionContext",
		})
	}
	return errors
}

// Merge merges this context into the target context
func (ldc *LoggingDecisionContext) Merge(target ChainableContext) ChainableContext {
	if target == nil {
		return ldc
	}

	targetLDC, ok := target.(*LoggingDecisionContext)
	if !ok {
		return ldc
	}

	// Determine precedence
	if ldc.precedence < targetLDC.precedence {
		// This context has higher precedence
		return &LoggingDecisionContext{
			LoggingCtx:   ldc.LoggingCtx,
			SecurityCtx:  ldc.SecurityCtx,
			MCPCtx:       ldc.MCPCtx,
			OperationCtx: ldc.OperationCtx,
			Component:    ldc.Component,
			Operation:    ldc.Operation,
			ClientID:     ldc.ClientID,
			TraceEnabled: ldc.TraceEnabled || targetLDC.TraceEnabled,
			Verbose:      ldc.Verbose || targetLDC.Verbose,
			precedence:   ldc.precedence,
			depth:        ldc.depth,
		}
	}

	// Target has higher precedence
	return &LoggingDecisionContext{
		LoggingCtx:   targetLDC.LoggingCtx,
		SecurityCtx:  targetLDC.SecurityCtx,
		MCPCtx:       targetLDC.MCPCtx,
		OperationCtx: targetLDC.OperationCtx,
		Component:    targetLDC.Component,
		Operation:    targetLDC.Operation,
		ClientID:     targetLDC.ClientID,
		TraceEnabled: ldc.TraceEnabled || targetLDC.TraceEnabled,
		Verbose:      ldc.Verbose || targetLDC.Verbose,
		precedence:   targetLDC.precedence,
		depth:        targetLDC.depth,
	}
}

// loggingDecisionContextKey is a private type for context keys
type loggingDecisionContextKey struct{}

// WithLoggingDecisionContext adds LoggingDecisionContext to the Go context
func WithLoggingDecisionContext(ctx context.Context, ldc *LoggingDecisionContext) context.Context {
	if ldc == nil {
		return ctx
	}
	return context.WithValue(ctx, loggingDecisionContextKey{}, ldc)
}

// GetLoggingDecisionContext retrieves LoggingDecisionContext from the Go context
func GetLoggingDecisionContext(ctx context.Context) *LoggingDecisionContext {
	if ldc, ok := ctx.Value(loggingDecisionContextKey{}).(*LoggingDecisionContext); ok {
		return ldc
	}
	return nil
}
