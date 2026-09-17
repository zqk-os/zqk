package mcp

import (
	"bytes"
	"maps"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// cmdResultKey* are JSON keys for MCP command result payloads (MCP wire shape).
const (
	cmdResultKeyArgs           = "args"
	cmdResultKeyErrorCode      = "error_code"
	cmdResultKeyErrorType      = "error_type"
	cmdResultKeyExecutionError = "execution_error"
	cmdResultKeyStderr         = "stderr"
	cmdResultKeyStdout         = "stdout"
	cmdResultKeySuccess        = "success"
)

// CommandResultBuilder provides a fluent API for building CLI command result responses
// This standardizes command result structure and integrates with error codes
type CommandResultBuilder struct {
	result      map[string]any
	commandPath string
	cmdArgs     []string
	stdout      bytes.Buffer
	stderr      bytes.Buffer
	execErr     error
	success     bool
}

// NewCommandResultBuilder creates a new command result builder
func NewCommandResultBuilder(commandPath string, cmdArgs []string) *CommandResultBuilder {
	return &CommandResultBuilder{
		result:      make(map[string]any),
		commandPath: commandPath,
		cmdArgs:     cmdArgs,
		success:     true, // Default to success
	}
}

// WithResult sets the base result data (from command output parsing)
func (b *CommandResultBuilder) WithResult(result map[string]any) *CommandResultBuilder {
	// Merge result data into builder
	maps.Copy(b.result, result)
	return b
}

// WithStdout sets the stdout buffer
func (b *CommandResultBuilder) WithStdout(stdout bytes.Buffer) *CommandResultBuilder {
	b.stdout = stdout
	return b
}

// WithStderr sets the stderr buffer
func (b *CommandResultBuilder) WithStderr(stderr bytes.Buffer) *CommandResultBuilder {
	b.stderr = stderr
	return b
}

// WithError sets the execution error
func (b *CommandResultBuilder) WithError(err error) *CommandResultBuilder {
	b.execErr = err
	b.success = false
	return b
}

// WithSuccess marks the command as successful
func (b *CommandResultBuilder) WithSuccess(success bool) *CommandResultBuilder {
	b.success = success
	return b
}

// WithErrorCode adds an error code to the result
// This integrates with the error codes scheme
func (b *CommandResultBuilder) WithErrorCode(code int) *CommandResultBuilder {
	b.result[cmdResultKeyErrorCode] = code
	return b
}

// WithErrorType adds an error type identifier
func (b *CommandResultBuilder) WithErrorType(errorType string) *CommandResultBuilder {
	b.result[cmdResultKeyErrorType] = errorType
	return b
}

// WithData adds custom data to the result
func (b *CommandResultBuilder) WithData(key string, value any) *CommandResultBuilder {
	b.result[key] = value
	return b
}

// Build creates the final command result map
func (b *CommandResultBuilder) Build() map[string]any {
	// Add metadata
	b.result[objects.FieldKeyCommand] = b.commandPath
	b.result[cmdResultKeyArgs] = b.cmdArgs
	b.result[cmdResultKeySuccess] = b.success

	// Handle error case
	if b.execErr != nil {
		b.result[cmdResultKeyExecutionError] = b.execErr.Error()
		b.result[cmdResultKeySuccess] = false

		// Include stderr if available
		if b.stderr.Len() > 0 {
			b.result[cmdResultKeyStderr] = b.stderr.String()
		}

		// Include stdout if available (may contain partial output)
		if b.stdout.Len() > 0 {
			b.result[cmdResultKeyStdout] = b.stdout.String()
		}

		// If no error code set, try to infer from error
		if _, hasCode := b.result[cmdResultKeyErrorCode]; !hasCode {
			b.inferErrorCodeFromError()
		}
	}

	return b.result
}

// inferErrorCodeFromError attempts to infer error code from execution error
func (b *CommandResultBuilder) inferErrorCodeFromError() {
	if b.execErr == nil {
		return
	}

	errMsg := b.execErr.Error()

	// Check for common error patterns
	switch {
	case commandResultContainsAny(errMsg, []string{"permission denied", "access denied", "forbidden"}):
		b.result[cmdResultKeyErrorCode] = PermissionDenied
		b.result[cmdResultKeyErrorType] = "permission_denied"
	case commandResultContainsAny(errMsg, []string{"not found", "does not exist", "no such"}):
		b.result[cmdResultKeyErrorCode] = NotFound
		b.result[cmdResultKeyErrorType] = "not_found"
	case commandResultContainsAny(errMsg, []string{"already exists", "duplicate", "conflict"}):
		b.result[cmdResultKeyErrorCode] = AlreadyExists
		b.result[cmdResultKeyErrorType] = "already_exists"
	case commandResultContainsAny(errMsg, []string{"invalid", "malformed", "bad format"}):
		b.result[cmdResultKeyErrorCode] = InvalidParameter
		b.result[cmdResultKeyErrorType] = "invalid_parameter"
	case commandResultContainsAny(errMsg, []string{"timeout", "timed out"}):
		b.result[cmdResultKeyErrorCode] = OperationTimeout
		b.result[cmdResultKeyErrorType] = "operation_timeout"
	default:
		// Default to internal error for unknown errors
		b.result[cmdResultKeyErrorCode] = InternalError
		b.result[cmdResultKeyErrorType] = "internal_error"
	}
}

// commandResultContainsAny checks if the string contains any of the substrings (case-insensitive)
func commandResultContainsAny(s string, substrings []string) bool {
	sLower := strings.ToLower(s)
	for _, substr := range substrings {
		if strings.Contains(sLower, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// NewCommandResultFromError creates a command result builder for error cases
// This provides a convenient way to create error results with proper error codes
func NewCommandResultFromError(commandPath string, cmdArgs []string, err error, errorCode int) *CommandResultBuilder {
	return NewCommandResultBuilder(commandPath, cmdArgs).
		WithError(err).
		WithErrorCode(errorCode)
}

// NewCommandResultFromSuccess creates a command result builder for success cases
func NewCommandResultFromSuccess(commandPath string, cmdArgs []string, result map[string]any) *CommandResultBuilder {
	return NewCommandResultBuilder(commandPath, cmdArgs).
		WithResult(result).
		WithSuccess(true)
}
