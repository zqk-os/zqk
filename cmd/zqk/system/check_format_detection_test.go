package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestFormatDetection_RealCommandSimulation tests format detection
// by simulating how the real command parses flags
func TestFormatDetection_RealCommandSimulation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-format-detection.jsonl")

	// Create command exactly like the real check command
	cmd := NewCheckCmd()

	// Set flags the way the real command would (via command line parsing)
	cmd.SetArgs([]string{"--format", "jsonl", "--output", outputPath})

	// Parse flags (this is what happens in real execution)
	if err := cmd.ParseFlags([]string{"--format", "jsonl", "--output", outputPath}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Create context
	ctx := cli.ContextForProjectAndProfile(tmpDir, "test")

	// Create test results
	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "backlog_item",
			FilePath:   "test/path1.yaml",
			Issues: []Issue{
				{
					Tier:     1,
					Category: "validation",
					Message:  "Test issue 1",
				},
			},
		},
		{
			ObjectID:   "TEST-002",
			ObjectKind: "backlog_item",
			FilePath:   "test/path2.yaml",
			Issues: []Issue{
				{
					Tier:     2,
					Category: "reference",
					Message:  "Test issue 2",
				},
			},
		},
	}

	// Call outputResults (this is where format detection happens)
	err := outputResults(cmd, ctx, results, nil, nil, nil)
	if err != nil {
		t.Fatalf("outputResults failed: %v", err)
	}

	// Read and validate output
	data, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Output file is empty")
	}

	// Check format
	firstLine := strings.TrimSpace(strings.Split(string(data), "\n")[0])

	// Should be JSONL (one object per line), NOT JSON with summary
	if strings.HasPrefix(firstLine, "{\n  \"summary\"") || strings.HasPrefix(firstLine, "{\"summary\"") {
		t.Error("❌ Output is JSON format (with summary) instead of JSONL!")
		t.Logf("First 500 chars:\n%s", string(data[:min(500, len(data))]))
		return
	}

	// Should be valid JSONL (one CheckResult per line)
	var firstResult CheckResult
	if err := json.Unmarshal([]byte(firstLine), &firstResult); err != nil {
		t.Errorf("First line is not valid JSONL: %v", err)
		t.Logf("First line: %s", firstLine)
		return
	}

	if firstResult.ObjectID == emptyValue {
		t.Error("First line doesn't have object_id - not a valid CheckResult")
		return
	}

	// Count valid lines
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	validLines := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}
		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.ObjectID != emptyValue {
			validLines++
		}
	}

	if validLines != len(results) {
		t.Errorf("Expected %d valid JSONL lines, got %d", len(results), validLines)
		previewLen := 1000
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("Output content:\n%s", string(data[:previewLen]))
	}

	t.Logf("✅ Format detection working - found %d valid JSONL lines", validLines)
}

// TestFormatDetection_FlagChangedState tests that format flag Changed state
// is correctly detected
func TestFormatDetection_FlagChangedState(t *testing.T) {
	t.Parallel()
	cmd := NewCheckCmd()

	// Test 1: Flag not set (should use default/context)
	formatFlag := cmd.Flag("format")
	if formatFlag == nil {
		t.Fatal("Format flag not found")
	}

	// Initially, flag should not be changed
	if formatFlag.Changed {
		t.Error("Format flag should not be changed initially")
	}

	// Test 2: Set flag via ParseFlags (simulates command line)
	cmd.SetArgs([]string{"--format", "jsonl"})
	if err := cmd.ParseFlags([]string{"--format", "jsonl"}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	// Now flag should be changed
	if !formatFlag.Changed {
		t.Error("Format flag should be changed after ParseFlags")
	}

	// Test 3: Check format detection logic
	formatStr := formatFlag.Value.String()
	if formatStr != "jsonl" {
		t.Errorf("Expected format 'jsonl', got %q", formatStr)
	}

	// Test 4: Check if handler is found
	format := cli.OutputFormat(formatStr)
	handler := cli.GetFormatHandler(format)
	if handler == nil {
		t.Error("JSONL format handler not found")
	}

	if !handler.IsStreaming() {
		t.Error("JSONL handler should be streaming")
	}

	t.Logf("✅ Format detection logic working: format=%q, handler=%v, streaming=%v",
		formatStr, handler != nil, handler != nil && handler.IsStreaming())
}

// TestFormatDetection_HandlerPath tests that the correct handler path is used
func TestFormatDetection_HandlerPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-handler-path.jsonl")

	cmd := NewCheckCmd()
	cmd.SetArgs([]string{"--format", "jsonl", "--output", outputPath})
	if err := cmd.ParseFlags([]string{"--format", "jsonl", "--output", outputPath}); err != nil {
		t.Fatalf("Failed to parse flags: %v", err)
	}

	ctx := cli.ContextForProjectAndProfile(tmpDir, "test")

	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "backlog_item",
			FilePath:   "test/path1.yaml",
			Issues:     []Issue{},
		},
	}

	// Check format detection before calling outputResults
	formatFlag := cmd.Flag("format")
	var formatStr string
	if formatFlag != nil && formatFlag.Changed {
		formatStr = formatFlag.Value.String()
	} else {
		format := cli.GetFormat(cmd)
		formatStr = string(format)
	}
	format := cli.OutputFormat(formatStr)
	handler := cli.GetFormatHandler(format)

	t.Logf("Format detection: formatStr=%q, format=%q, handler=%v, IsStreaming=%v",
		formatStr, string(format), handler != nil,
		func() bool {
			if handler != nil {
				return handler.IsStreaming()
			}
			return false
		}())

	if handler == nil {
		t.Fatal("Handler not found - format detection failed!")
	}

	if !handler.IsStreaming() {
		t.Error("Handler should be streaming for JSONL format")
	}

	// Now call outputResults
	err := outputResults(cmd, ctx, results, nil, nil, nil)
	if err != nil {
		t.Fatalf("outputResults failed: %v", err)
	}

	// Verify output
	data, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Should be JSONL, not JSON with summary
	if strings.Contains(string(data), "\"summary\"") {
		t.Error("❌ Output contains 'summary' - using wrong format (JSON instead of JSONL)")
		t.Logf("Output:\n%s", string(data))
	}

	// Should be valid JSONL
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	validCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}
		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.ObjectID != emptyValue {
			validCount++
		}
	}

	if validCount != len(results) {
		t.Errorf("Expected %d valid JSONL objects, got %d", len(results), validCount)
		t.Logf("Output:\n%s", string(data))
	}

	t.Logf("✅ Handler path working correctly - %d valid JSONL objects", validCount)
}
