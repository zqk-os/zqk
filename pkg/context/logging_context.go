package context

import (
	"context"
	"io"
)

// LoggingProfile represents the logging profile/context
type LoggingProfile string

const (
	// ProfileMCP is for MCP server mode (stderr, JSON, Debug)
	ProfileMCP LoggingProfile = "mcp"
	// ProfileSystem is for system logs (stderr, JSON, Debug)
	ProfileSystem LoggingProfile = "system"
	// ProfileAIAgent is for AI agent mode (stdout, JSON, Debug)
	ProfileAIAgent LoggingProfile = "ai-agent"
	// ProfileDebug is for debug mode (stdout, Compact, Debug)
	ProfileDebug LoggingProfile = "debug"
	// ProfileHuman is for human-readable mode (stdout, Text, Info)
	ProfileHuman LoggingProfile = "human"
	emptyProfile LoggingProfile = ""
)

// LoggingContext represents logging configuration
// Implements ChainableContext for chain-based processing
type LoggingContext struct {
	// Profile determines the logging profile (mcp, system, ai-agent, debug, human)
	Profile LoggingProfile

	// Level is the minimum log level (can override profile default)
	// If nil, uses profile default
	Level *int // Using *int to allow nil (use profile default)

	// SuppressDebugToStdout prevents debug logs from going to stdout
	// This is useful for preventing stdout pollution in human-readable mode
	SuppressDebugToStdout bool

	// Chain support
	precedence int
	depth      int
}

// NewLoggingContext creates a new LoggingContext with a profile
func NewLoggingContext(profile LoggingProfile) *LoggingContext {
	return &LoggingContext{
		Profile:               profile,
		Level:                 nil, // Use profile default
		SuppressDebugToStdout: false,
		precedence:            PrecedenceDefault,
		depth:                 DepthRoot,
	}
}

// NewSystemLoggingContext creates a LoggingContext for system operations
// System logs should never pollute stdout
func NewSystemLoggingContext() *LoggingContext {
	return &LoggingContext{
		Profile:               ProfileSystem,
		Level:                 nil,
		SuppressDebugToStdout: true, // System logs should never go to stdout
		precedence:            PrecedenceSystem,
		depth:                 DepthRoot,
	}
}

// NewHumanLoggingContext creates a LoggingContext for human-readable output
// Suppresses debug logs to stdout to prevent pollution
func NewHumanLoggingContext() *LoggingContext {
	return &LoggingContext{
		Profile:               ProfileHuman,
		Level:                 nil,
		SuppressDebugToStdout: true, // Prevent debug logs from polluting stdout
		precedence:            PrecedenceDefault,
		depth:                 DepthRoot,
	}
}

// WithSuppressDebugToStdout sets whether to suppress debug logs to stdout
func (l *LoggingContext) WithSuppressDebugToStdout(suppress bool) *LoggingContext {
	l.SuppressDebugToStdout = suppress
	return l
}

// WithLevel sets the log level (overrides profile default)
func (l *LoggingContext) WithLevel(level int) *LoggingContext {
	l.Level = &level
	return l
}

// GetPrecedence returns the precedence level for this context
func (l *LoggingContext) GetPrecedence() int {
	return l.precedence
}

// SetPrecedence sets the precedence level for this context
func (l *LoggingContext) SetPrecedence(precedence int) {
	l.precedence = precedence
}

// GetDepth returns the depth of this context in the hierarchy
func (l *LoggingContext) GetDepth() int {
	return l.depth
}

// SetDepth sets the depth of this context in the hierarchy
func (l *LoggingContext) SetDepth(depth int) {
	l.depth = depth
}

// Validate performs validation on this context node
func (l *LoggingContext) Validate() []ValidationError {
	var errors []ValidationError
	if l.Profile == emptyProfile {
		errors = append(errors, ValidationError{
			Field:   "Profile",
			Message: "logging profile cannot be empty",
			Context: "LoggingContext",
		})
	}
	if l.Level != nil && *l.Level < 0 {
		errors = append(errors, ValidationError{
			Field:   "Level",
			Message: "log level cannot be negative",
			Context: "LoggingContext",
		})
	}
	return errors
}

// Merge merges this context into the target context
// Higher precedence context wins for profile and level
// SuppressDebugToStdout is OR'd (if either suppresses, result suppresses)
func (l *LoggingContext) Merge(target ChainableContext) ChainableContext {
	if target == nil {
		return l
	}

	targetLogging, ok := target.(*LoggingContext)
	if !ok {
		// Can't merge with non-LoggingContext, return self
		return l
	}

	// Determine precedence
	if l.precedence < targetLogging.precedence {
		// This context has higher precedence, use its values
		return &LoggingContext{
			Profile:               l.Profile,
			Level:                 l.Level,
			SuppressDebugToStdout: l.SuppressDebugToStdout || targetLogging.SuppressDebugToStdout,
			precedence:            l.precedence,
			depth:                 l.depth,
		}
	}

	// Target has higher precedence, use its values but merge SuppressDebugToStdout
	return &LoggingContext{
		Profile:               targetLogging.Profile,
		Level:                 targetLogging.Level,
		SuppressDebugToStdout: l.SuppressDebugToStdout || targetLogging.SuppressDebugToStdout,
		precedence:            targetLogging.precedence,
		depth:                 targetLogging.depth,
	}
}

// GetWriter returns the appropriate writer for this logging context
// This encapsulates the logic for determining stdout vs stderr
func (l *LoggingContext) GetWriter() io.Writer {
	// This would need to import os, but we'll keep it simple for now
	// The actual implementation would be in pkg/logging
	return nil // Placeholder - actual implementation in pkg/logging
}

// ShouldSuppressDebugToStdout returns true if debug logs should be suppressed from stdout
func (l *LoggingContext) ShouldSuppressDebugToStdout() bool {
	return l.SuppressDebugToStdout
}

// loggingContextKey is a private type for context keys to avoid collisions
type loggingContextKey struct{}

// WithLoggingContext adds LoggingContext to the Go context
func WithLoggingContext(ctx context.Context, loggingCtx *LoggingContext) context.Context {
	if loggingCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, loggingContextKey{}, loggingCtx)
}

// GetLoggingContext retrieves LoggingContext from the Go context
// Returns nil if no LoggingContext is present
func GetLoggingContext(ctx context.Context) *LoggingContext {
	if loggingCtx, ok := ctx.Value(loggingContextKey{}).(*LoggingContext); ok {
		return loggingCtx
	}
	return nil
}
