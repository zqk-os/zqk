package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCheckCommand_JSONLOutput_Quick is a faster test that checks the actual output file
// from a previous run to detect corruption patterns
func TestCheckCommand_JSONLOutput_Quick(t *testing.T) {
	t.Parallel()
	// Find the project root and check the actual output file
	cwd, _ := fileutil.Getwd() //nolint:errcheck // Test helper - current directory is sufficient //nolint:errcheck // Test helper - error handling not critical
	// Try to find project root by looking for test-scenarios directory
	var outputPath string
	for dir := cwd; dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "test-scenarios", "check-violation-resolver", "check-original.jsonl")
		if _, err := fileutil.Stat(candidate); err == nil {
			outputPath = candidate
			break
		}
	}

	if outputPath == emptyValue {
		// Try relative path from current directory
		outputPath = "test-scenarios/check-violation-resolver/check-original.jsonl"
		if _, err := fileutil.Stat(outputPath); fileutil.IsNotExist(err) {
			t.Skipf("Output file not found - run check command first to create: test-scenarios/check-violation-resolver/check-original.jsonl")
			return
		}
	}

	// Read the file
	data, err := fileutil.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Output file is empty")
	}

	t.Logf("Analyzing file: %s (%d bytes)", outputPath, len(data))

	// Check for corrupted first line (just "{")
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		t.Fatal("File has no lines")
	}

	firstLine := strings.TrimSpace(lines[0])
	if firstLine == "{" {
		t.Error("❌ CORRUPTION DETECTED: First line is incomplete JSON (just '{')")
		t.Logf("First line: %q", firstLine)
		if len(lines) > 1 {
			previewLen := 200
			if len(lines[1]) < previewLen {
				previewLen = len(lines[1])
			}
			t.Logf("Second line (first 200 chars): %s", lines[1][:previewLen])
		}
	}

	// Check if it's JSON format (with summary) instead of JSONL
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{\n  \"summary\"") {
		t.Error("❌ FORMAT ERROR: Output is in JSON format (with summary) instead of JSONL format!")
		previewLen := 500
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("First 500 chars:\n%s", string(data[:previewLen]))
	}

	// Analyze lines
	validLines := 0
	corruptedLines := 0
	summaryLines := 0

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}

		// Check for incomplete JSON
		if line == "{" {
			corruptedLines++
			if corruptedLines == 1 {
				t.Errorf("Line %d is corrupted (just '{')", i+1)
			}
			continue
		}

		// Check for summary object
		if strings.HasPrefix(line, "{\"summary\"") || strings.HasPrefix(line, "{\n  \"summary\"") {
			summaryLines++
			continue
		}

		// Try to parse as CheckResult
		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			// Might be buffer info or other metadata - that's okay
			continue
		}

		// Verify it has object_id
		if result.ObjectID != emptyValue {
			validLines++
		}
	}

	// Report findings
	t.Logf("File analysis:")
	t.Logf("  Total lines: %d", len(lines))
	t.Logf("  Valid CheckResult objects: %d", validLines)
	t.Logf("  Corrupted lines (just '{'): %d", corruptedLines)
	t.Logf("  Summary/metadata lines: %d", summaryLines)

	if corruptedLines > 0 {
		t.Errorf("❌ Found %d corrupted lines - this is the bug we're tracking!", corruptedLines)
	}

	if validLines == 0 && len(data) > 100 {
		t.Error("❌ No valid CheckResult objects found in output!")
		previewLen := 1000
		if len(data) < previewLen {
			previewLen = len(data)
		}
		t.Logf("First 1000 chars:\n%s", string(data[:previewLen]))
	}

	// Check first few valid lines
	if validLines > 0 {
		validCount := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == emptyValue || line == "{" {
				continue
			}
			var result CheckResult
			if err := json.Unmarshal([]byte(line), &result); err == nil && result.ObjectID != emptyValue {
				validCount++
				if validCount <= 3 {
					t.Logf("  Valid object %d: %s (%s)", validCount, result.ObjectID, result.ObjectKind)
				}
			}
		}
	}
}
