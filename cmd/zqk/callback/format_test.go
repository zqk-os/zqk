package callback

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProcessor_FormatJSONL(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.jsonl")

	payload := map[string]any{
		"job_id":                     "TEST-001",
		objects.FieldKeyCallbackType: "completion",
		"success":                    true,
		"duration":                   1.5,
		objects.FieldKeyCommand:      "echo test",
		"stdout":                     "test output",
	}

	err := p.ProcessDirect(tempDir, logFile, true, payload, "jsonl")
	if err != nil {
		t.Fatalf("failed to process: %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Verify it's valid JSONL (one JSON object per line)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line in JSONL, got %d", len(lines))
	}

	// Verify it's valid JSON
	var event map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &event); err != nil {
		t.Fatalf("failed to parse JSONL: %v", err)
	}

	// Verify required fields
	if event["job_id"] != "TEST-001" {
		t.Errorf("expected job_id TEST-001, got %v", event["job_id"])
	}
	if event[objects.FieldKeyCallbackType] != "completion" {
		t.Errorf("expected callback_type completion, got %v", event[objects.FieldKeyCallbackType])
	}
	if _, ok := event["timestamp"]; !ok {
		t.Error("expected timestamp field in JSONL output")
	}
}

func TestProcessor_FormatJSON(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.json")

	payload := map[string]any{
		"job_id":                     "TEST-002",
		objects.FieldKeyCallbackType: "completion",
		"success":                    true,
		"duration":                   2.0,
		objects.FieldKeyCommand:      "echo test2",
	}

	err := p.ProcessDirect(tempDir, logFile, true, payload, "json")
	if err != nil {
		t.Fatalf("failed to process: %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Verify it's valid JSON (should be pretty-printed with indentation)
	var event map[string]any
	if err := json.Unmarshal(content, &event); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Verify required fields
	if event["job_id"] != "TEST-002" {
		t.Errorf("expected job_id TEST-002, got %v", event["job_id"])
	}

	// JSON format should be pretty-printed (contains newlines and spaces)
	contentStr := string(content)
	if !strings.Contains(contentStr, "\n") {
		t.Error("JSON format should be pretty-printed with newlines")
	}
}

func TestProcessor_FormatText(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	payload := map[string]any{
		"job_id":                     "TEST-003",
		objects.FieldKeyCallbackType: "completion",
		"success":                    true,
		"duration":                   1.0,
		objects.FieldKeyCommand:      "echo test3",
	}

	err := p.ProcessDirect(tempDir, logFile, true, payload, "text")
	if err != nil {
		t.Fatalf("failed to process: %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	contentStr := string(content)
	// Verify text format contains human-readable formatting
	if !strings.Contains(contentStr, "TEST-003") {
		t.Error("expected text format to contain job ID")
	}
	if !strings.Contains(contentStr, "Status: success") {
		t.Error("expected text format to contain status")
	}
	if !strings.Contains(contentStr, "Command:") {
		t.Error("expected text format to contain command label")
	}
}

func TestProcessor_MultipleFormatsInSameFile(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "events.jsonl")

	// Process multiple entries in JSONL format (should append)
	payload1 := map[string]any{
		"job_id":                     "TEST-001",
		objects.FieldKeyCallbackType: "completion",
		"success":                    true,
	}
	payload2 := map[string]any{
		"job_id":                     "TEST-002",
		objects.FieldKeyCallbackType: "error",
		"success":                    false,
	}

	err := p.ProcessDirect(tempDir, logFile, true, payload1, "jsonl")
	if err != nil {
		t.Fatalf("failed to process first entry: %v", err)
	}

	err = p.ProcessDirect(tempDir, logFile, true, payload2, "jsonl")
	if err != nil {
		t.Fatalf("failed to process second entry: %v", err)
	}

	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Should have 2 lines (JSONL format)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines in JSONL, got %d", len(lines))
	}

	// Verify both are valid JSON
	for i, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to parse JSONL line %d: %v", i+1, err)
		}
	}
}

func TestProcessor_FormatLogEntry_InvalidFormat(t *testing.T) {
	t.Parallel()
	p := NewProcessorForTest()
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	// Initialize with invalid format - should default to text
	err := p.Initialize(tempDir, logFile, true, 100, &TimestampSorter{}, "invalid-format")
	if err != nil {
		t.Fatalf("failed to initialize: %v", err)
	}
	defer p.Shutdown()

	payload := map[string]any{
		"job_id": "TEST-001",
	}

	err = p.Enqueue(payload)
	if err != nil {
		t.Fatalf("failed to enqueue: %v", err)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Should still write (defaults to text format)
	content, err := fileutil.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if len(content) == 0 {
		t.Error("expected log file to have content even with invalid format")
	}
}
