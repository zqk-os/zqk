package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// TestLogger_Progress tests the Progress method
func TestLogger_Progress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		formatter ProgressFormatter
		checkFunc func(t *testing.T, output string)
	}{
		{
			name:      "TextProgressFormatter",
			formatter: NewTextProgressFormatter(),
			checkFunc: func(t *testing.T, output string) {
				// Remove \r for checking (progress bars use \r for overwriting)
				output = strings.ReplaceAll(output, "\r", "")
				if !strings.Contains(output, "[") || !strings.Contains(output, "]") {
					t.Errorf("Text progress should contain progress bar, got: %q", output)
				}
				if !strings.Contains(output, "50%") {
					t.Errorf("Text progress should contain percentage, got: %q", output)
				}
			},
		},
		{
			name:      "JSONProgressFormatter",
			formatter: NewJSONProgressFormatter(),
			checkFunc: func(t *testing.T, output string) {
				// JSON formatter outputs newline, so we need to trim it
				output = strings.TrimSpace(output)
				var data map[string]any
				if err := json.Unmarshal([]byte(output), &data); err != nil {
					t.Fatalf("JSON progress should be valid JSON: %v, got: %q", err, output)
				}
				if data[objectFieldKeyType] != "progress" {
					t.Errorf("Expected type='progress', got: %v", data[objectFieldKeyType])
				}
				if data[objectFieldKeyOperationID] != "op-123" {
					t.Errorf("Expected operation_id='op-123', got: %v", data[objectFieldKeyOperationID])
				}
				if data["progress"] != float64(50) {
					t.Errorf("Expected progress=50, got: %v", data["progress"])
				}
			},
		},
		{
			name:      "CompactProgressFormatter",
			formatter: NewCompactProgressFormatter(),
			checkFunc: func(t *testing.T, output string) {
				// Remove \r for checking (compact formatter uses \r for overwriting)
				output = strings.ReplaceAll(output, "\r", "")
				if !strings.Contains(output, "op-123") {
					t.Errorf("Compact progress should contain operation ID, got: %q", output)
				}
				if !strings.Contains(output, "50%") {
					t.Errorf("Compact progress should contain percentage, got: %q", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

			// Create a progress logger with the formatter and explicit writer
			// Use NewProgressLoggerWithWriter to ensure output goes to the buffer
			progressLogger := NewProgressLoggerWithWriter(logger, tt.formatter, &buf)

			progressLogger.Progress("op-123", 50, "Processing",
				String("object_id", "BLI-001"),
				String("operation_type", "create"))

			output := buf.String()
			tt.checkFunc(t, output)
		})
	}
}

// TestLogger_Status tests the Status method
func TestLogger_Status(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressLogger := NewProgressLogger(logger, NewTextProgressFormatter())

	progressLogger.Status("op-123", "pending", "in_progress",
		String("object_id", "BLI-001"))

	output := buf.String()
	if !strings.Contains(output, "pending") || !strings.Contains(output, "in_progress") {
		t.Error("Status output should contain status transition")
	}
	if !strings.Contains(output, "BLI-001") {
		t.Error("Status output should contain object ID")
	}
}

// TestLogger_Stream tests the Stream method
func TestLogger_Stream(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressLogger := NewProgressLogger(logger, NewJSONProgressFormatter())

	ctx := pkgctx.NewSystemContext()
	streamData := map[string]any{
		objectFieldKeyEventType: "operation.progress",
		"progress":              75,
	}

	err := progressLogger.Stream(ctx, "operation.progress", streamData,
		String("object_id", "BLI-001"))
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	output := buf.String()
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		t.Fatalf("Stream output should be valid JSON: %v", err)
	}

	if data[objectFieldKeyType] != "stream_event" {
		t.Errorf("Expected type='stream_event', got: %v", data[objectFieldKeyType])
	}
	if data[objectFieldKeyEventType] != "operation.progress" {
		t.Errorf("Expected event_type='operation.progress', got: %v", data[objectFieldKeyEventType])
	}
}

// TestProgressLogger_OverwriteBehavior tests line overwriting behavior
func TestProgressLogger_OverwriteBehavior(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// Text formatter supports overwrite
	textProgressLogger := NewProgressLogger(logger, NewTextProgressFormatter())

	// Multiple progress updates should overwrite (use \r)
	textProgressLogger.Progress("op-123", 25, "Starting", String("object_id", "BLI-001"))
	textProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))
	textProgressLogger.Progress("op-123", 75, "Almost done", String("object_id", "BLI-001"))

	output := buf.String()
	// With overwrite, we should see multiple lines (each with \r)
	// The last update should be present
	if !strings.Contains(output, "75%") {
		t.Error("Last progress update should be present")
	}

	// JSON formatter should not overwrite (each update is separate)
	var jsonBuf bytes.Buffer
	jsonLogger := NewLogger(&jsonBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	jsonProgressLogger := NewProgressLogger(jsonLogger, NewJSONProgressFormatter())

	jsonProgressLogger.Progress("op-123", 25, "Starting", String("object_id", "BLI-001"))
	jsonProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	jsonOutput := jsonBuf.String()
	lines := strings.Split(strings.TrimSpace(jsonOutput), "\n")
	if len(lines) < 2 {
		t.Error("JSON progress should output separate lines for each update")
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
	}
}

// TestProgressLogger_ContextAware tests context-aware formatter selection
func TestProgressLogger_ContextAware(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// Test that formatter is selected based on context
	// For now, we'll test that the formatter passed to NewProgressLogger is used
	textProgressLogger := NewProgressLogger(logger, NewTextProgressFormatter())
	textProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	output := buf.String()
	// Text formatter should produce human-readable output (not JSON)
	if strings.Contains(output, "\"type\"") {
		t.Error("Text formatter should not produce JSON")
	}

	// JSON formatter should produce JSON
	var jsonBuf bytes.Buffer
	jsonLogger := NewLogger(&jsonBuf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	jsonProgressLogger := NewProgressLogger(jsonLogger, NewJSONProgressFormatter())
	jsonProgressLogger.Progress("op-123", 50, "Processing", String("object_id", "BLI-001"))

	jsonOutput := jsonBuf.String()
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &data); err != nil {
		t.Errorf("JSON formatter should produce valid JSON: %v", err)
	}
}

// TestProgressLogger_WithFields tests that fields are passed to formatter
func TestProgressLogger_WithFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	progressLogger := NewProgressLogger(logger, NewJSONProgressFormatter())

	progressLogger.Progress("op-123", 50, "Processing",
		String("object_id", "BLI-001"),
		String("operation_type", "create"),
		Int("items_processed", 42))

	output := buf.String()
	var data map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &data); err != nil {
		t.Fatalf("Progress output should be valid JSON: %v", err)
	}

	if data["object_id"] != "BLI-001" {
		t.Errorf("Expected object_id='BLI-001', got: %v", data["object_id"])
	}
	if data["operation_type"] != "create" {
		t.Errorf("Expected operation_type='create', got: %v", data["operation_type"])
	}
	if data["items_processed"] != float64(42) {
		t.Errorf("Expected items_processed=42, got: %v", data["items_processed"])
	}
}
// tdd refresh
