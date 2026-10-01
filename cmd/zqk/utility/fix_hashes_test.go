package utility

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCollectFilesByKind(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory structure
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	backlogDir := filepath.Join(processDir, "backlog_items")

	// Create test files
	if err := fileutil.MkdirAll(backlogDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create a test backlog item file
	testFile := filepath.Join(backlogDir, "BLI-001.yaml")
	testContent := `id: BLI-001
kind: backlog_item
title: Test Item
`
	if err := fileutil.WriteFile(testFile, []byte(testContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a hash registry file (should be skipped)
	hashFile := filepath.Join(backlogDir, ".backlog_item.hashes")
	if err := fileutil.WriteFile(hashFile, []byte("{}"), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create hash file: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	files, errors := collectFilesByKind(tmpDir, "backlog_item", logger, true)

	if len(errors) > 0 {
		t.Errorf("Unexpected errors: %v", errors)
	}

	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}

	if files[0] != testFile {
		t.Errorf("Expected file %s, got %s", testFile, files[0])
	}
}

func TestCollectInternalFiles(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory structure
	tmpDir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	auditDir := filepath.Join(processDir, "audit", "2026-01")
	changeDir := filepath.Join(processDir, "change_journal_entries", "2026-01")

	// Create test directories
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create audit directory: %v", err)
	}
	if err := fileutil.MkdirAll(changeDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create change_journal directory: %v", err)
	}

	// Create test files
	auditFile := filepath.Join(auditDir, "AUD-001.yaml")
	auditContent := `id: AUD-001
kind: audit_event
event_type: command_execution
`
	if err := fileutil.WriteFile(auditFile, []byte(auditContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create audit file: %v", err)
	}

	changeFile := filepath.Join(changeDir, "CHA-001.yaml")
	changeContent := `id: CHA-001
kind: change_journal_entry
operation: create
`
	if err := fileutil.WriteFile(changeFile, []byte(changeContent), paths.FilePerm644); err != nil { //nolint:gosec // Test files - 0600 is acceptable
		t.Fatalf("Failed to create change journal file: %v", err)
	}

	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	files, errors := collectInternalFiles(tmpDir, logger, true)

	if len(errors) > 0 {
		t.Errorf("Unexpected errors: %v", errors)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestFixHashesWithKindFlag(t *testing.T) {
	t.Parallel()
	// This test would require a full CLI context setup
	// For now, we test the helper functions directly
	t.Skip("Requires full CLI context - integration test needed")
}

func TestGetPrefixFromID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id     string
		prefix string
	}{
		{"ADR-001", "ADR-"},
		{"BLI-123", "BLI-"},
		{"AUD-456", "AUD-"},
		{"NO-DASH", "NO-"}, // Function extracts up to first dash
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := getPrefixFromID(tt.id)
			if got != tt.prefix {
				t.Errorf("getPrefixFromID(%q) = %q, want %q", tt.id, got, tt.prefix)
			}
		})
	}
}

func TestReadAndParseYAMLFile_ErrorWrapping(t *testing.T) {
	t.Parallel()
	// Test file read error unwrapping
	nonExistentFile := filepath.Join(t.TempDir(), "non-existent-file.yaml")
	_, _, err := readAndParseYAMLFile(nonExistentFile)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected errors.Is(err, os.ErrNotExist) to be true for wrapped error, got false; err: %v", err)
	}

	// Test invalid yaml parse error
	invalidFile := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := fileutil.WriteFile(invalidFile, []byte("invalid:\n  - incomplete: ["), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write invalid yaml file: %v", err)
	}
	_, _, err = readAndParseYAMLFile(invalidFile)
	if err == nil {
		t.Fatal("expected parse error for invalid yaml, got nil")
	}
}
