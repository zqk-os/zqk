package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// handleToolsCall handles the tools/call method
func (s *Server) handleToolsCall(ctx context.Context, _ string, params json.RawMessage) (any, error) {
	if !s.initialized.Load() {
		return nil, &JSONRPCError{
			Code:    NotInitialized,
			Message: "Not initialized",
		}
	}

	// Ensure tool execution cannot hang: use bounded context when request has no deadline
	ctx = EnsureContext(ctx)
	if ctx.Err() != nil {
		s.traceLogf("[MCP_ERROR] tools/call request ctx already done before exec: err=%v", ctx.Err())
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, MaxToolCallDuration)
		defer cancel()
	}

	var toolParams ToolCallParams
	if err := json.Unmarshal(params, &toolParams); err != nil {
		return nil, &JSONRPCError{
			Code:    ParseError,
			Message: "Parse error",
		}
	}

	// Get client ID with role prefix for logging
	clientIDWithRole := s.getClientIDWithRole()
	clientTag := emptyValue
	if clientIDWithRole != emptyValue {
		clientTag = fmt.Sprintf(" client_id=%s", clientIDWithRole)
	}

	// Log tool call start to trace file
	if s.getTraceWriter() != nil {
		s.traceLogf("[MCP_TRACE] → Tool call started: tool=%s%s", toolParams.Name, clientTag)
	}

	// Pass context to handleToolCall for cancellation support
	startTime := time.Now()
	result, err := s.handleToolCallWithContext(ctx, toolParams.Name, toolParams.Arguments)
	duration := time.Since(startTime)

	// Record tools_call event after completion (so we can include duration and success status)
	eventCtx := s.getClientEventContext()
	fields := map[string]any{
		"tool":     toolParams.Name,
		"duration": duration.String(),
		"success":  err == nil,
	}
	if err != nil {
		fields["error"] = err.Error()
	}
	eventCtx.RecordEvent("tools_call", fields)

	// Log tool call completion to trace file
	if err != nil {
		// Check if this is an authorization error (expected) vs actual execution error
		errorStr := err.Error()
		isAuthError := strings.Contains(errorStr, "Command not allowed") ||
			strings.Contains(errorStr, "Write operation not allowed") ||
			strings.Contains(errorStr, "Format not allowed")

		if isAuthError {
			// Authorization errors are expected - log at debug level without ✗ symbol
			s.traceLogf("[MCP_DEBUG] → Tool call denied: tool=%s, duration=%v, reason=%v%s",
				toolParams.Name, duration, err, clientTag)
		} else {
			// Actual execution errors - log as failures
			if s.getTraceWriter() != nil {
				s.traceLogf("[MCP_ERROR] ✗ Tool call failed: tool=%s, duration=%v, error=%v%s",
					toolParams.Name, duration, err, clientTag)
			}
		}
	} else {
		s.traceLogf("[MCP_TRACE] ← Tool call completed: tool=%s, duration=%v%s",
			toolParams.Name, duration, clientTag)
	}
	if err != nil {
		// Check if it's an ElicitationError - these should be returned as errors, not wrapped in ToolCallResult
		// This allows the server.go error handling to convert them to JSON-RPC errors with elicitation data
		elicitationError := &ElicitationError{}
		if errors.As(err, &elicitationError) {
			return nil, err // Return elicitation error directly so it gets proper JSON-RPC error treatment
		}

		// Check if result contains detailed error information (from CLI command execution)
		// The CLI bridge returns detailed error info in the result map even when err != nil
		if resultMap, ok := result.(map[string]any); ok {
			// Include detailed error information from the result
			errorText := fmt.Sprintf("Error: %v", err)
			if execErr, ok := resultMap["execution_error"].(string); ok {
				errorText = execErr
			}

			// Build detailed error message with available information
			var details []string
			if stderr, ok := resultMap["stderr"].(string); ok && stderr != emptyValue {
				details = append(details, fmt.Sprintf("stderr: %s", stderr))
			}
			if stdout, ok := resultMap["stdout"].(string); ok && stdout != emptyValue {
				details = append(details, fmt.Sprintf("stdout: %s", stdout))
			}
			if parseErr, ok := resultMap["parse_error"].(string); ok && parseErr != emptyValue {
				details = append(details, fmt.Sprintf("parse_error: %s", parseErr))
			}

			if len(details) > 0 {
				errorText = fmt.Sprintf("%s\n%s", errorText, strings.Join(details, "\n"))
			}

			// Return error as tool result with IsError flag (MCP spec allows this)
			// Include the full result map for debugging
			// Determine format from result if available, otherwise use default
			formatHint := emptyValue
			if format, ok := resultMap[objects.FieldKeyFormat].(string); ok {
				formatHint = format
			}
			return ToolCallResult{
				Content: s.buildToolCallContent(errorText, formatHint),
				IsError: true,
			}, nil
		}

		// Fallback: Return simple error message if result is not a map
		return ToolCallResult{
			Content: s.buildToolCallContent(fmt.Sprintf("Error: %v", err), emptyValue),
			IsError: true,
		}, nil
	}

	// Marshal result to JSON string
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return nil, &JSONRPCError{
			Code:    InternalError,
			Message: fmt.Sprintf("failed to marshal result: %v", err),
		}
	}

	// Determine format from result if available (CLI commands may return format info)
	formatHint := emptyValue
	if resultMap, ok := result.(map[string]any); ok {
		if format, ok := resultMap[objects.FieldKeyFormat].(string); ok {
			formatHint = format
		}
	}

	// Build content using config-driven content type and MIME type detection
	return ToolCallResult{
		Content: s.buildToolCallContent(string(resultJSON), formatHint),
	}, nil
}
