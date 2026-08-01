package logging

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestProgressFormatterInterface tests the ProgressFormatter interface
// This ensures all formatters implement the required methods correctly
func TestProgressFormatterInterface(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		formatter ProgressFormatter
	}{
		{"TextProgressFormatter", NewTextProgressFormatter()},
		{"JSONProgressFormatter", NewJSONProgressFormatter()},
		{"CompactProgressFormatter", NewCompactProgressFormatter()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test SupportsOverwrite
			supportsOverwrite := tt.formatter.SupportsOverwrite()
			if tt.name == "TextProgressFormatter" && !supportsOverwrite {
				t.Errorf("TextProgressFormatter should support overwrite")
			}
			if tt.name == "JSONProgressFormatter" && supportsOverwrite {
				t.Errorf("JSONProgressFormatter should not support overwrite")
			}

			// Test FormatProgress
			fields := map[string]any{
				"object_id":      "ITEM-001",
				"operation_type": "create",
			}
			progress, err := tt.formatter.FormatProgress("op-123", 50, "Processing...", fields)
			if err != nil {
				t.Errorf("FormatProgress failed: %v", err)
			}
			if len(progress) == 0 {
				t.Errorf("FormatProgress returned empty output")
			}

			// Test FormatStatus
			status, err := tt.formatter.FormatStatus("op-123", "pending", "in_progress", fields)
			if err != nil {
				t.Errorf("FormatStatus failed: %v", err)
			}
			if len(status) == 0 {
				t.Errorf("FormatStatus returned empty output")
			}

			// Test FormatStreamEvent
			streamData := map[string]any{
				objectFieldKeyEventType: "operation.progress",
				"progress":              50,
			}
			stream, err := tt.formatter.FormatStreamEvent("operation.progress", streamData, fields)
			if err != nil {
				t.Errorf("FormatStreamEvent failed: %v", err)
			}
			if len(stream) == 0 {
				t.Errorf("FormatStreamEvent returned empty output")
			}
		})
	}
}

// TestTextProgressFormatter tests the text progress formatter
func TestTextProgressFormatter(t *testing.T) {
	t.Parallel()
	formatter := NewTextProgressFormatter()

	if !formatter.SupportsOverwrite() {
		t.Error("TextProgressFormatter should support overwrite")
	}

	// Test progress formatting
	fields := map[string]any{
		"object_id": "ITEM-001",
	}
	progress, err := formatter.FormatProgress("op-123", 75, "Processing object", fields)
	if err != nil {
		t.Fatalf("FormatProgress failed: %v", err)
	}

	progressStr := string(progress)
	// Should contain progress bar
	if !strings.Contains(progressStr, "[") || !strings.Contains(progressStr, "]") {
		t.Errorf("Progress output should contain progress bar, got: %s", progressStr)
	}
	// Should contain message
	if !strings.Contains(progressStr, "Processing object") {
		t.Errorf("Progress output should contain message, got: %s", progressStr)
	}
	// Should contain object ID
	if !strings.Contains(progressStr, "ITEM-001") {
		t.Errorf("Progress output should contain object_id, got: %s", progressStr)
	}

	// Test status formatting
	status, err := formatter.FormatStatus("op-123", "pending", "in_progress", fields)
	if err != nil {
		t.Fatalf("FormatStatus failed: %v", err)
	}

	statusStr := string(status)
	// Should contain status transition
	if !strings.Contains(statusStr, "pending") || !strings.Contains(statusStr, "in_progress") {
		t.Errorf("Status output should contain status transition, got: %s", statusStr)
	}
}

// TestJSONProgressFormatter tests the JSON progress formatter
func TestJSONProgressFormatter(t *testing.T) {
	t.Parallel()
	formatter := NewJSONProgressFormatter()

	if formatter.SupportsOverwrite() {
		t.Error("JSONProgressFormatter should not support overwrite")
	}

	// Test progress formatting
	fields := map[string]any{
		"object_id":      "ITEM-001",
		"operation_type": "create",
	}
	progress, err := formatter.FormatProgress("op-123", 75, "Processing object", fields)
	if err != nil {
		t.Fatalf("FormatProgress failed: %v", err)
	}

	// Should be valid JSON
	var data map[string]any
	if err := json.Unmarshal(progress, &data); err != nil {
		t.Fatalf("Progress output should be valid JSON: %v, got: %s", err, string(progress))
	}

	// Should contain required fields
	if data[objectFieldKeyType] != "progress" {
		t.Errorf("Expected type='progress', got: %v", data[objectFieldKeyType])
	}
	if data[objectFieldKeyOperationID] != "op-123" {
		t.Errorf("Expected operation_id='op-123', got: %v", data[objectFieldKeyOperationID])
	}
	if data["progress"] != float64(75) {
		t.Errorf("Expected progress=75, got: %v", data["progress"])
	}
	if data["message"] != "Processing object" {
		t.Errorf("Expected message='Processing object', got: %v", data["message"])
	}
	if data["object_id"] != "ITEM-001" {
		t.Errorf("Expected object_id='ITEM-001', got: %v", data["object_id"])
	}

	// Test status formatting
	status, err := formatter.FormatStatus("op-123", "pending", "in_progress", fields)
	if err != nil {
		t.Fatalf("FormatStatus failed: %v", err)
	}

	var statusData map[string]any
	if err := json.Unmarshal(status, &statusData); err != nil {
		t.Fatalf("Status output should be valid JSON: %v, got: %s", err, string(status))
	}

	if statusData[objectFieldKeyType] != "status" {
		t.Errorf("Expected type='status', got: %v", statusData[objectFieldKeyType])
	}
	if statusData["old_status"] != "pending" {
		t.Errorf("Expected old_status='pending', got: %v", statusData["old_status"])
	}
	if statusData["new_status"] != "in_progress" {
		t.Errorf("Expected new_status='in_progress', got: %v", statusData["new_status"])
	}

	// Test stream event formatting
	streamData := map[string]any{
		objectFieldKeyEventType: "operation.progress",
		"progress":              50,
	}
	stream, err := formatter.FormatStreamEvent("operation.progress", streamData, fields)
	if err != nil {
		t.Fatalf("FormatStreamEvent failed: %v", err)
	}

	var streamEvent map[string]any
	if err := json.Unmarshal(stream, &streamEvent); err != nil {
		t.Fatalf("Stream event should be valid JSON: %v, got: %s", err, string(stream))
	}

	if streamEvent[objectFieldKeyType] != "stream_event" {
		t.Errorf("Expected type='stream_event', got: %v", streamEvent[objectFieldKeyType])
	}
	if streamEvent[objectFieldKeyEventType] != "operation.progress" {
		t.Errorf("Expected event_type='operation.progress', got: %v", streamEvent[objectFieldKeyEventType])
	}
}

// TestCompactProgressFormatter tests the compact progress formatter
func TestCompactProgressFormatter(t *testing.T) {
	t.Parallel()
	formatter := NewCompactProgressFormatter()

	if !formatter.SupportsOverwrite() {
		t.Error("CompactProgressFormatter should support overwrite")
	}

	// Test progress formatting
	fields := map[string]any{
		"object_id": "ITEM-001",
	}
	progress, err := formatter.FormatProgress("op-123", 50, "Processing", fields)
	if err != nil {
		t.Fatalf("FormatProgress failed: %v", err)
	}

	progressStr := string(progress)
	// Should be compact (single line)
	if strings.Count(progressStr, "\n") > 1 {
		t.Errorf("Compact progress should be single line, got: %s", progressStr)
	}
	// Should contain progress percentage
	if !strings.Contains(progressStr, "50%") {
		t.Errorf("Progress output should contain percentage, got: %s", progressStr)
	}
}

// TestProgressFormatterLineOverwrite tests line overwriting behavior
func TestProgressFormatterLineOverwrite(t *testing.T) {
	t.Parallel()
	textFormatter := NewTextProgressFormatter()
	jsonFormatter := NewJSONProgressFormatter()
	compactFormatter := NewCompactProgressFormatter()

	// Text and compact should support overwrite
	if !textFormatter.SupportsOverwrite() {
		t.Error("TextProgressFormatter should support overwrite")
	}
	if !compactFormatter.SupportsOverwrite() {
		t.Error("CompactProgressFormatter should support overwrite")
	}

	// JSON should not support overwrite (each update is separate)
	if jsonFormatter.SupportsOverwrite() {
		t.Error("JSONProgressFormatter should not support overwrite")
	}
}

// TestProgressFormatterContextAware tests that formatters handle context correctly
func TestProgressFormatterContextAware(t *testing.T) {
	t.Parallel()
	textFormatter := NewTextProgressFormatter()
	jsonFormatter := NewJSONProgressFormatter()

	fields := map[string]any{
		"object_id":           "ITEM-001",
		objectFieldKeyContext: "human", // Human context
	}

	// Text formatter should produce human-readable output
	textOutput, err := textFormatter.FormatProgress("op-123", 50, "Processing", fields)
	if err != nil {
		t.Fatalf("FormatProgress failed: %v", err)
	}
	textStr := string(textOutput)
	if strings.Contains(textStr, "\"type\"") {
		t.Error("Text formatter should not produce JSON for human context")
	}

	// JSON formatter should always produce JSON
	jsonOutput, err := jsonFormatter.FormatProgress("op-123", 50, "Processing", fields)
	if err != nil {
		t.Fatalf("FormatProgress failed: %v", err)
	}
	var jsonData map[string]any
	if err := json.Unmarshal(jsonOutput, &jsonData); err != nil {
		t.Errorf("JSON formatter should always produce valid JSON: %v", err)
	}
}
