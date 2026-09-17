package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/spf13/cobra"
)

func TestOutputResults_JSONLFormat(t *testing.T) {
	// Create a temporary file for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-output.jsonl")

	// Create a mock command with format and output flags
	cmd := &cobra.Command{}
	cmd.Flags().String("format", "jsonl", "")
	cmd.Flags().String("output", outputPath, "")
	cmd.Flags().Bool("verbose", false, "")
	// Mark format flag as changed so format detection works correctly
	_ = cmd.Flags().Set("format", "jsonl")

	// Create a mock context
	ctx := cli.ContextForProjectAndProfile(tmpDir, "test")

	// Create test check results
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

	// Call outputResults
	err := outputResults(cmd, ctx, results, nil, nil, nil)
	if err != nil && !isCompletedSystemCheckError(err) {
		t.Fatalf("outputResults failed: %v", err)
	}

	// Read the output file
	data, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Verify it's not empty
	if len(data) == 0 {
		t.Fatal("Output file is empty")
	}

	// Check for corrupted first line (just "{")
	firstLine := strings.Split(string(data), "\n")[0]
	if strings.TrimSpace(firstLine) == "{" {
		t.Error("First line is corrupted (just '{') - this is the bug we're tracking down!")
		previewLen := 200
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("First 200 chars of output: %s", string(data[:previewLen]))
	}

	// Check if it's JSON format (with summary) instead of JSONL
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{\n  \"summary\"") {
		t.Error("Output is in JSON format (with summary) instead of JSONL format!")
		previewLen := 500
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("First 500 chars of output: %s", string(data[:previewLen]))
	}

	// Split into lines
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Verify each line is valid JSON with object_id
	validLines := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}

		// Check for incomplete JSON (just "{")
		if line == "{" {
			t.Errorf("Line %d is incomplete JSON (just '{')", i+1)
			continue
		}

		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			// Might be buffer info or other metadata - skip for now
			continue
		}

		// Verify it has object_id
		if result.ObjectID != emptyValue {
			validLines++
			if result.ObjectID != results[validLines-1].ObjectID {
				t.Errorf("Line %d: expected object_id %s, got %s", i+1, results[validLines-1].ObjectID, result.ObjectID)
			}
		}
	}

	if validLines == 0 {
		t.Error("No valid CheckResult objects found in output!")
		previewLen := 1000
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("Output content (first 1000 chars):\n%s", string(data[:previewLen]))
	}

	if validLines != len(results) {
		t.Errorf("Expected %d valid result lines, got %d", len(results), validLines)
	}

	t.Logf("✓ Successfully wrote %d valid JSONL lines", validLines)
}

func TestOutputResults_FormatDetection(t *testing.T) {
	// Test that format detection works correctly
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-output.jsonl")

	cmd := &cobra.Command{}
	cmd.Flags().String("format", "jsonl", "")
	cmd.Flags().String("output", outputPath, "")
	cmd.Flags().Bool("verbose", false, "")

	// Mark format flag as changed
	formatFlag := cmd.Flag("format")
	if formatFlag != nil {
		formatFlag.Changed = true
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

	err := outputResults(cmd, ctx, results, nil, nil, nil)
	if err != nil {
		t.Fatalf("outputResults failed: %v", err)
	}

	// Verify the file was created and is JSONL format
	data, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Should not start with "{\n  \"summary\"" (JSON format)
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{\n  \"summary\"") {
		t.Error("Format detection failed - output is JSON instead of JSONL")
	}

	// Should be valid JSONL (one object per line)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	validCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue || line == "{" {
			continue
		}
		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err == nil && result.ObjectID != emptyValue {
			validCount++
		}
	}

	if validCount == 0 {
		t.Error("No valid JSONL lines found")
		t.Logf("Output: %s", string(data))
	}

	t.Logf("✓ Format detection working - found %d valid JSONL lines", validCount)
}
