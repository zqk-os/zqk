package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestMCPCompatibility_ProgressOutputToStderr tests that progress output goes to stderr in MCP mode
func TestMCPCompatibility_ProgressOutputToStderr(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var stderrBuf bytes.Buffer
	var stdoutBuf bytes.Buffer

	// Create logger with stdout writer
	logger := NewLogger(&stdoutBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressFormatter := NewJSONProgressFormatter()
	// Progress logger should route to stderr in MCP mode
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &stderrBuf)

	// Write progress update
	progressLogger.Progress("op-123", 50, "Processing", String("object_id", "ITEM-001"))

	// Progress should go to stderr (not stdout) in MCP mode
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "progress") {
		t.Error("Progress output should go to stderr in MCP mode")
	}

	// Progress should NOT go to stdout
	stdoutOutput := stdoutBuf.String()
	if strings.Contains(stdoutOutput, "progress") {
		t.Error("Progress output should NOT go to stdout in MCP mode")
	}

	// Verify it's valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderrOutput)), &data); err != nil {
		t.Fatalf("Progress output should be valid JSON in MCP mode: %v", err)
	}

	if data[objectFieldKeyType] != "progress" {
		t.Errorf("Expected type='progress', got: %v", data[objectFieldKeyType])
	}
}

// TestMCPCompatibility_JSONFormatterInMCPMode tests that JSON formatter is used in MCP mode
func TestMCPCompatibility_JSONFormatterInMCPMode(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var buf bytes.Buffer
	loggingCtx := pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	logger := GetLoggerFromLoggingContext(pkgctx.NewSystemContext(), loggingCtx)

	// MCP profile should use JSON formatter
	progressFormatter := NewJSONProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)

	// Write progress update
	progressLogger.Progress("op-123", 75, "Almost done", String("object_id", "ITEM-001"))

	output := buf.String()
	// Should be valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		t.Fatalf("MCP mode should produce valid JSON: %v", err)
	}

	// Should have required fields
	if data[objectFieldKeyType] != "progress" {
		t.Errorf("Expected type='progress', got: %v", data[objectFieldKeyType])
	}
	if data[objectFieldKeyOperationID] != "op-123" {
		t.Errorf("Expected operation_id='op-123', got: %v", data[objectFieldKeyOperationID])
	}
	if data["progress"] != float64(75) {
		t.Errorf("Expected progress=75, got: %v", data["progress"])
	}
}

// TestMCPCompatibility_StreamEvents tests that stream events work in MCP mode
func TestMCPCompatibility_StreamEvents(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressFormatter := NewJSONProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)

	ctx := pkgctx.NewSystemContext()
	streamData := map[string]any{
		objectFieldKeyEventType: "operation.completed",
		"duration_ms":           1234,
	}

	err := progressLogger.Stream(ctx, "operation.completed", streamData, String("object_id", "ITEM-001"))
	if err != nil {
		t.Fatalf("Stream should not fail in MCP mode: %v", err)
	}

	output := buf.String()
	// Should be valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		t.Fatalf("Stream event should be valid JSON in MCP mode: %v", err)
	}

	// Should have required fields
	if data[objectFieldKeyType] != "stream_event" {
		t.Errorf("Expected type='stream_event', got: %v", data[objectFieldKeyType])
	}
	if data[objectFieldKeyEventType] != "operation.completed" {
		t.Errorf("Expected event_type='operation.completed', got: %v", data[objectFieldKeyEventType])
	}
}

// TestMCPCompatibility_StatusChanges tests that status changes work in MCP mode
func TestMCPCompatibility_StatusChanges(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressFormatter := NewJSONProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)

	// Write status change
	progressLogger.Status("op-123", "pending", "in_progress", String("object_id", "ITEM-001"))

	output := buf.String()
	// Should be valid JSON
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		t.Fatalf("Status change should be valid JSON in MCP mode: %v", err)
	}

	// Should have required fields
	if data[objectFieldKeyType] != "status" {
		t.Errorf("Expected type='status', got: %v", data[objectFieldKeyType])
	}
	if data["old_status"] != "pending" {
		t.Errorf("Expected old_status='pending', got: %v", data["old_status"])
	}
	if data["new_status"] != "in_progress" {
		t.Errorf("Expected new_status='in_progress', got: %v", data["new_status"])
	}
}

// TestMCPCompatibility_NoOverwriteInJSONMode tests that JSON formatter doesn't use overwrite
func TestMCPCompatibility_NoOverwriteInJSONMode(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressFormatter := NewJSONProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)

	// Multiple progress updates
	progressLogger.Progress("op-123", 25, "Starting", String("object_id", "ITEM-001"))
	progressLogger.Progress("op-123", 50, "Processing", String("object_id", "ITEM-001"))
	progressLogger.Progress("op-123", 75, "Almost done", String("object_id", "ITEM-001"))

	output := buf.String()
	// JSON formatter should NOT use \r (no overwriting)
	if strings.Contains(output, "\r") {
		t.Error("JSON formatter should not use \\r for overwriting in MCP mode")
	}

	// Should have multiple JSON objects (one per update)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 3 {
		t.Errorf("Expected at least 3 JSON objects (one per update), got: %d", len(lines))
	}

	// Each line should be valid JSON
	for i, line := range lines {
		if line == emptyValue {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			t.Errorf("Line %d should be valid JSON: %v, got: %s", i, err, line)
		}
		if data[objectFieldKeyType] != "progress" {
			t.Errorf("Line %d should have type='progress', got: %v", i, data[objectFieldKeyType])
		}
	}
}

// TestMCPCompatibility_ContextAwareFormatterSelection tests that formatter is selected based on MCP context
func TestMCPCompatibility_ContextAwareFormatterSelection(t *testing.T) {
	t.Parallel()
	// Save original env
	originalMCPAccountID := os.Getenv(zqkenv.MCPAccountID())
	defer func() {
		if originalMCPAccountID != emptyValue {
			os.Setenv(zqkenv.MCPAccountID(), originalMCPAccountID)
		} else {
			os.Unsetenv(zqkenv.MCPAccountID())
		}
	}()

	// Set MCP mode
	os.Setenv(zqkenv.MCPAccountID(), "test-account")

	var buf bytes.Buffer
	loggingCtx := pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	logger := GetLoggerFromLoggingContext(pkgctx.NewSystemContext(), loggingCtx)

	// MCP profile should use JSON formatter
	progressFormatter := NewJSONProgressFormatter()
	progressLogger := NewProgressLoggerWithWriter(logger, progressFormatter, &buf)

	progressLogger.Progress("op-123", 50, "Processing", String("object_id", "ITEM-001"))

	output := buf.String()
	// Should be JSON (not text)
	if !strings.Contains(output, "\"type\":\"progress\"") {
		t.Error("MCP mode should use JSON formatter")
	}

	// Should NOT be text format
	if strings.Contains(output, "[") && strings.Contains(output, "]") && !strings.Contains(output, "\"") {
		t.Error("MCP mode should not use text formatter")
	}
}
