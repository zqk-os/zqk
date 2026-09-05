package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/objects"
)

// isCriticalError checks if an error is critical based on error codes
// This is more reliable than string matching and can be configured externally
// Checks JSONRPCError codes first, then falls back to configurable critical error codes
func (s *Server) isCriticalError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a JSONRPCError and use its code

	// Check if error wraps a JSONRPCError (common pattern)
	var jsonrpcErr *JSONRPCError
	if errors.As(err, &jsonrpcErr) {
		return s.isCriticalErrorCode(jsonrpcErr.Code)
	}

	// For non-JSONRPCError errors, we can't determine criticality from code
	// Return false to avoid false positives (string matching was too broad)
	return false
}

// isCriticalErrorCode checks if an error code is considered critical
// Uses the structured error code system from error_codes.go
func (s *Server) isCriticalErrorCode(code int) bool {
	// First check if config has custom critical error codes
	criticalCodes := s.getCriticalErrorCodes()
	if len(criticalCodes) > 0 {
		for _, criticalCode := range criticalCodes {
			if code == criticalCode {
				return true
			}
		}
		return false
	}

	// Use the structured error code system's criticality determination
	return IsCriticalErrorCode(code)
}

// getCriticalErrorCodes returns the list of critical error codes from config
// This allows external configuration of which error codes are considered critical
func (s *Server) getCriticalErrorCodes() []int {
	if s.config == nil || len(s.config.MCPServer.ErrorHandling.CriticalErrorCodes) == 0 {
		return nil // Use defaults
	}
	return s.config.MCPServer.ErrorHandling.CriticalErrorCodes
}

// SendCriticalError sends a critical error notification with helpful instructions
// This provides better user experience by sending actionable error messages via notifications
// instead of just failing silently or with cryptic error codes
// severity: "error", "warning", "critical", "fatal"
// category: e.g., "authentication", "authorization", "storage", "configuration", "tool_execution"
// clientID: optional client ID to route notification to specific client (empty string = use current/default client)
// suppressNotification: if true, only logs the error without sending a notification message
//
//	This is useful for errors that occur during initialization or in contexts where
//	notifications aren't appropriate (e.g., expected validation errors)
func (s *Server) SendCriticalError(err error, severity, category, message string, instructions []string, fields map[string]any, clientID string, suppressNotification bool) error {
	if err == nil {
		return nil
	}

	// Build fields for logging (always log, even if notification is suppressed)
	logFields := make(map[string]any)
	maps.Copy(logFields, fields)
	logFields[objects.FieldKeyCategory] = category
	logFields[objects.FieldKeySeverity] = severity
	logFields["error"] = err.Error()
	if clientID != emptyValue {
		logFields[objects.FieldKeyClientID] = clientID
	}

	// Determine log level based on severity
	var logLevel LogLevel
	switch strings.ToLower(severity) {
	case "fatal", "critical":
		logLevel = LogLevelError
	case "error":
		logLevel = LogLevelError
	case "warning":
		logLevel = LogLevelWarn
	default:
		logLevel = LogLevelError
	}

	// Always send log message (logging is context-independent)
	_ = s.SendLogMessage(logLevel, fmt.Sprintf("%s error: %s", category, err.Error()), logFields) //nolint:errcheck // Log failures are non-critical

	// Check if we should send notification message
	// Suppress if explicitly requested or if context doesn't allow notifications
	if suppressNotification || !s.canSendNotifications() {
		// Only log, don't send notification
		return nil
	}

	// Persist critical error to cap_failure_tracker.json for failure observability
	if s.initCtx != nil && s.initCtx.ProjectRoot != "" {
		trackerPath := filepath.Join(s.initCtx.ProjectRoot, paths.ProjectDataDir, "state", "cap_failure_tracker.json")
		fileutil.

			// Ensure directory exists
			EnsureDir(filepath.Dir(trackerPath))

		// Define anonymous struct matching capFailureTracker
		var tracker struct {
			ConsecutiveFailures int    `json:"consecutive_failures"`
			LastFailure         string `json:"last_failure"`
			LastStage           string `json:"last_stage"`
			LastError           string `json:"last_error"`
			AgentEscalations    int    `json:"agent_escalations"`
			LastAgentEscalation string `json:"last_agent_escalation,omitempty"`
			LastHumanEscalation string `json:"last_human_escalation,omitempty"`
		}

		// Read existing tracker if available
		if data, err := fileutil.ReadFile(trackerPath); err == nil {
			_ = json.Unmarshal(data, &tracker)
		}

		// Update tracker with this critical error
		tracker.ConsecutiveFailures++
		tracker.LastFailure = time.Now().UTC().Format(time.RFC3339)
		tracker.LastStage = "mcp_server"
		tracker.LastError = err.Error()

		// Write back
		if data, marshalErr := json.MarshalIndent(tracker, "", "  "); marshalErr == nil {
			_ = fileutil.WriteSecureFile(trackerPath, data)
		}
	}

	// Build comprehensive error message for notification
	var fullMessage strings.Builder
	fmt.Fprintf(&fullMessage, "⚠️ %s Error: %s\n\n", strings.ToUpper(category), message)
	fmt.Fprintf(&fullMessage, "Error details: %s\n\n", err.Error())

	if len(instructions) > 0 {
		fullMessage.WriteString("To resolve this issue:\n")
		for i, instruction := range instructions {
			fmt.Fprintf(&fullMessage, "%d. %s\n", i+1, instruction)
		}
		fullMessage.WriteString("\n")
	}

	fullMessage.WriteString("The server will continue operating, but some operations may be limited.")

	// Send notification message to specific client or default to current connection
	messageType := fmt.Sprintf("%s_error", category)
	priority := severity
	if priority == emptyValue {
		priority = "high"
	}

	// Route to specific client if clientID provided, otherwise use default routing
	if clientID != emptyValue {
		_ = s.SendMessageToClientByID(clientID, fullMessage.String(), messageType, priority) //nolint:errcheck // Notification failures are non-critical
	} else {
		_ = s.SendMessageToClient(fullMessage.String(), messageType, priority) //nolint:errcheck // Notification failures are non-critical
	}

	return nil
}
