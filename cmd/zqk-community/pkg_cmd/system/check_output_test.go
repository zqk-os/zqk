package system

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func TestOutputJSONL(t *testing.T) {
	t.Parallel()
	// Create a temporary file for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-output.jsonl")

	var c cobra.Command
	cmd := &c
	cmd.Flags().String("output", outputPath, "")
	cmd.Flags().String("format", "jsonl", "")

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

	// Call outputJSONL
	err := outputJSONL(cmd, results, 0, nil)
	if err != nil {
		t.Fatalf("outputJSONL failed: %v", err)
	}

	// Read the output file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Verify it's not empty
	if len(data) == 0 {
		t.Fatal("Output file is empty")
	}

	// Split into lines
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(results) {
		t.Fatalf("Expected %d lines, got %d", len(results), len(lines))
	}

	// Verify each line is valid JSON with object_id
	for i, line := range lines {
		if strings.TrimSpace(line) == emptyValue {
			t.Errorf("Line %d is empty", i+1)
			continue
		}

		// Check for incomplete JSON (just "{")
		if strings.TrimSpace(line) == "{" {
			t.Errorf("Line %d is incomplete JSON (just '{')", i+1)
			continue
		}

		var result CheckResult
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			preview := line
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			t.Errorf("Line %d is not valid JSON: %v\nContent: %s", i+1, err, preview)
			continue
		}

		// Verify it has object_id
		if result.ObjectID == emptyValue {
			t.Errorf("Line %d missing object_id", i+1)
		}

		// Verify it matches expected result
		expected := results[i]
		if result.ObjectID != expected.ObjectID {
			t.Errorf("Line %d: expected object_id %s, got %s", i+1, expected.ObjectID, result.ObjectID)
		}
	}

	t.Logf("✓ Successfully wrote %d valid JSONL lines", len(lines))
}

func TestOutputJSONL_WithBufferInfo(t *testing.T) {
	t.Parallel()
	// Create a temporary file for output
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test-output-buffer.jsonl")

	var c cobra.Command
	cmd := &c
	cmd.Flags().String("output", outputPath, "")
	cmd.Flags().String("format", "jsonl", "")

	// Create test check results
	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "backlog_item",
			FilePath:   "test/path1.yaml",
			Issues:     []Issue{},
		},
	}

	bufferSummary := map[string]int{
		"audit_event": 5,
		"change":      3,
	}

	// Call outputJSONL with buffer info
	err := outputJSONL(cmd, results, 8, bufferSummary)
	if err != nil {
		t.Fatalf("outputJSONL failed: %v", err)
	}

	// Read the output file
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	// Split into lines
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	// Should have at least the result + buffer info
	if len(lines) < 2 {
		t.Fatalf("Expected at least 2 lines (result + buffer info), got %d", len(lines))
	}

	// Verify first line is the result
	var result CheckResult
	if err := json.Unmarshal([]byte(lines[0]), &result); err != nil {
		t.Fatalf("First line is not valid JSON: %v", err)
	}
	if result.ObjectID != "TEST-001" {
		t.Errorf("Expected object_id TEST-001, got %s", result.ObjectID)
	}

	// Verify last line is buffer info
	var bufferInfo map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &bufferInfo); err != nil {
		t.Fatalf("Last line (buffer info) is not valid JSON: %v\nContent: %s", err, lines[len(lines)-1])
	}

	if bufferInfo[objects.FieldKeyType] != "buffer_info" {
		t.Errorf("Expected buffer_info type, got %v", bufferInfo[objects.FieldKeyType])
	}

	t.Logf("✓ Successfully wrote result + buffer info in JSONL format")
}

func TestOutputJSONL_Stdout(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf)
	var c cobra.Command
	cmd := &c
	cmd.SetContext(ctx)
	cmd.Flags().String("format", "jsonl", "")

	// Create test check results
	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "backlog_item",
			FilePath:   "test/path1.yaml",
			Issues:     []Issue{},
		},
	}

	err := outputJSONL(cmd, results, 0, nil)
	if err != nil {
		t.Fatalf("outputJSONL failed: %v", err)
	}

	output := buf.String()
	// Verify output
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("Expected 1 line, got %d", len(lines))
	}

	// Verify it's valid JSON
	var result CheckResult
	if err := json.Unmarshal([]byte(lines[0]), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v\nContent: %s", err, lines[0])
	}

	if result.ObjectID != "TEST-001" {
		t.Errorf("Expected object_id TEST-001, got %s", result.ObjectID)
	}

	// Check for incomplete JSON
	if strings.TrimSpace(lines[0]) == "{" {
		t.Error("Output is incomplete JSON (just '{')")
	}

	t.Logf("✓ Successfully wrote to stdout in JSONL format")
}

func TestDispatchViolationsToInbox(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	results := []CheckResult{
		{
			ObjectID:   "TEST-001",
			ObjectKind: "backlog_item",
			FilePath:   "test/path1.yaml",
			Issues: []Issue{
				{
					Tier:        1,
					Category:    categoryPolicy,
					Message:     "Policy violation test",
					AutoFixable: false,
				},
			},
		},
		{
			ObjectID:   "TEST-002",
			ObjectKind: objects.KindTestCase,
			FilePath:   "test/path2.yaml",
			Issues: []Issue{
				{
					Tier:        2,
					Category:    categoryReference,
					Message:     "Reference issue test",
					AutoFixable: false,
				},
			},
		},
	}

	// 1. Dispatch with custom role "QA-Engineer"
	err := dispatchViolationsToInbox(tmpDir, results, roleQAEngineer)
	if err != nil {
		t.Fatalf("dispatchViolationsToInbox failed: %v", err)
	}

	// Check that files are written in tmpDir/.zqk/inbox/QA-Engineer/
	inboxDir := filepath.Join(tmpDir, ".zqk", "inbox", roleQAEngineer)
	files, err := os.ReadDir(inboxDir)
	if err != nil {
		t.Fatalf("Failed to read inbox dir: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("Expected 2 inbox files, got %d", len(files))
	}

	// 2. Dispatch with "auto" role mapping
	err = dispatchViolationsToInbox(tmpDir, results, dispatchAuto)
	if err != nil {
		t.Fatalf("dispatchViolationsToInbox auto failed: %v", err)
	}

	// For TEST-001 (policy), role should be "Security-Engineer"
	secDir := filepath.Join(tmpDir, ".zqk", "inbox", roleSecurityEngineer)
	secFiles, err := os.ReadDir(secDir)
	if err != nil {
		t.Fatalf("Failed to read Security-Engineer inbox dir: %v", err)
	}
	if len(secFiles) != 1 {
		t.Errorf("Expected 1 security inbox file, got %d", len(secFiles))
	}

	// For TEST-002 (reference / test_case), role should be "QA-Engineer"
	qaDir := filepath.Join(tmpDir, ".zqk", "inbox", roleQAEngineer)
	qaFiles, err := os.ReadDir(qaDir)
	if err != nil {
		t.Fatalf("Failed to read QA-Engineer inbox dir: %v", err)
	}
	// We dispatched it once to QA-Engineer in step 1, and once via auto in step 2. Total should be 3 files.
	if len(qaFiles) != 3 {
		t.Errorf("Expected 3 QA inbox files, got %d", len(qaFiles))
	}
}
