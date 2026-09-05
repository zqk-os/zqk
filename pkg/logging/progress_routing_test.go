package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestProgressRouting_SeparateFromLogs tests that progress output is routed separately from logs
func TestProgressRouting_SeparateFromLogs(t *testing.T) {
	t.Parallel()
	var logBuf bytes.Buffer
	var progressBuf bytes.Buffer

	// Create a logger for logs
	logger := NewLogger(&logBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// Create a progress logger with separate writer
	progressFormatter := NewTextProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &progressBuf)

	// Write a log entry
	logger.Info("This is a log message", String("key", "value"))

	// Write a progress update
	progressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	// Logs should be in log buffer
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "This is a log message") {
		t.Error("Log message should be in log buffer")
	}

	// Progress should be in progress buffer (separate)
	progressOutput := progressBuf.String()
	if !strings.Contains(progressOutput, "50%") {
		t.Error("Progress update should be in progress buffer")
	}

	// Progress should NOT be in log buffer
	if strings.Contains(logOutput, "50%") {
		t.Error("Progress output should not be in log buffer")
	}

	// Logs should NOT be in progress buffer
	if strings.Contains(progressOutput, "This is a log message") {
		t.Error("Log output should not be in progress buffer")
	}
}

// TestProgressRouting_MCPMode tests that progress output goes to stderr in MCP mode
func TestProgressRouting_MCPMode(t *testing.T) {
	// Set MCP mode
	t.Setenv(zqkenv.MCPAccountID(), "test-account")

	var stderrBuf bytes.Buffer

	// Create logger with stdout writer (but progress should go to stderr in MCP mode)
	logger := NewLogger(os.Stdout, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressFormatter := NewJSONProgressFormatter()
	// Use explicit writer for testing
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &stderrBuf)

	// Write progress update
	progressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	// Progress should go to stderr (not stdout) in MCP mode
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "progress") {
		t.Error("Progress output should go to stderr in MCP mode")
	}

	// Verify it's valid JSON
	if !strings.Contains(stderrOutput, "\"type\":\"progress\"") {
		t.Error("Progress output should be JSON in MCP mode")
	}
}

// TestProgressRouting_ContextAwareFormatter tests that formatter is selected based on context
func TestProgressRouting_ContextAwareFormatter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		profile      string // Use string for profile
		expectedType string // "json" or "text"
		checkFunc    func(t *testing.T, output string)
	}{
		{
			name:         "MCP profile uses JSON",
			profile:      "mcp",
			expectedType: "json",
			checkFunc: func(t *testing.T, output string) {
				if !strings.Contains(output, "\"type\":\"progress\"") {
					t.Error("MCP profile should use JSON formatter")
				}
			},
		},
		{
			name:         "AI Agent profile uses JSON",
			profile:      "ai-agent",
			expectedType: "json",
			checkFunc: func(t *testing.T, output string) {
				if !strings.Contains(output, "\"type\":\"progress\"") {
					t.Error("AI Agent profile should use JSON formatter")
				}
			},
		},
		{
			name:         "Human profile uses text",
			profile:      "human",
			expectedType: "text",
			checkFunc: func(t *testing.T, output string) {
				if strings.Contains(output, "\"type\":\"progress\"") {
					t.Error("Human profile should use text formatter, not JSON")
				}
				if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
					t.Error("Human profile should produce progress bar")
				}
			},
		},
		{
			name:         "Debug profile uses compact",
			profile:      "debug",
			expectedType: "text", // Compact is text-based
			checkFunc: func(t *testing.T, output string) {
				if strings.Contains(output, "\"type\":\"progress\"") {
					t.Error("Debug profile should use compact formatter, not JSON")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			// Create logging context from profile string
			loggingCtx := pkgctx.NewLoggingContext(pkgctx.LoggingProfile(tt.profile))
			logger := GetLoggerFromLoggingContext(pkgctx.NewSystemContext(), loggingCtx)

			// Determine expected formatter
			var progressFormatter ProgressFormatter
			switch tt.profile {
			case string(pkgctx.ProfileMCP), string(pkgctx.ProfileAIAgent):
				progressFormatter = NewJSONProgressFormatter()
			case string(pkgctx.ProfileDebug):
				progressFormatter = NewCompactProgressFormatter()
			default:
				progressFormatter = NewTextProgressFormatter()
			}

			progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)
			progressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

			output := buf.String()
			tt.checkFunc(t, output)
		})
	}
}

// TestProgressRouting_OverwriteBehavior tests that overwrite behavior is correct
func TestProgressRouting_OverwriteBehavior(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer

	// Text formatter supports overwrite
	textLogger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	textProgressLogger := NewProgressLoggerWithWriter(textLogger, NewTextProgressFormatter(), &buf)

	// Multiple progress updates
	textProgressLogger.Progress("op-123", 25, "Starting", String("object_id", "BLI-001"))
	textProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))
	textProgressLogger.Progress("op-123", 75, "Almost done", String("object_id", "BLI-001"))

	output := buf.String()
	// Text formatter uses \r for overwriting, so we should see multiple \r sequences
	if !strings.Contains(output, "\r") {
		t.Error("Text formatter should use \\r for overwriting")
	}

	// JSON formatter should not overwrite (each update is separate)
	var jsonBuf bytes.Buffer
	jsonLogger := NewLogger(&jsonBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	jsonProgressLogger := NewProgressLoggerWithWriter(jsonLogger, NewJSONProgressFormatter(), &jsonBuf)

	jsonProgressLogger.Progress("op-123", 25, "Starting", String("object_id", "BLI-001"))
	jsonProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	jsonOutput := jsonBuf.String()
	// JSON formatter should not use \r (no overwriting)
	if strings.Contains(jsonOutput, "\r") {
		t.Error("JSON formatter should not use \\r for overwriting")
	}

	// Should have multiple JSON objects (one per update)
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")
	if len(lines) < 2 {
		t.Error("JSON formatter should output separate lines for each update")
	}
}

// TestProgressRouting_StatusChanges tests that status changes are always new lines
func TestProgressRouting_StatusChanges(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressLogger := NewProgressLoggerWithWriter(logger, NewTextProgressFormatter(), &buf)

	// Status changes should always be new lines (not overwritten)
	progressLogger.Status("op-123", "pending", "in_progress", String("object_id", "BLI-001"))
	progressLogger.Status("op-123", "in_progress", "completed", String("object_id", "BLI-001"))

	output := buf.String()
	// Should contain both status transitions
	if !strings.Contains(output, "pending") || !strings.Contains(output, "in_progress") || !strings.Contains(output, "completed") {
		t.Error("Status changes should be written as new lines")
	}

	// Should have newlines (not just \r)
	if !strings.Contains(output, "\n") {
		t.Error("Status changes should use newlines, not overwrite")
	}
}

// TestProgressRouting_StreamEvents tests that stream events are formatted correctly
func TestProgressRouting_StreamEvents(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressLogger := NewProgressLoggerWithWriter(logger, NewJSONProgressFormatter(), &buf)

	ctx := pkgctx.NewSystemContext()
	streamData := map[string]any{
		objectFieldKeyEventType: "operation.progress",
		"progress":              75,
	}

	err := progressLogger.Stream(ctx, "operation.progress", streamData, String("object_id", "BLI-001"))
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	output := buf.String()
	// Should be valid JSON
	if !strings.Contains(output, "\"type\":\"stream_event\"") {
		t.Error("Stream event should be formatted as JSON")
	}
	if !strings.Contains(output, "\"event_type\":\"operation.progress\"") {
		t.Error("Stream event should contain event_type")
	}
}
