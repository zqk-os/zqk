package storage_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/storage"
)

// TestFindMisplacedObjectFiles_EmbeddedKindInExamples ensures that a top-level
// kind field (e.g. "kind: policy") is not masked by an indented "kind: <other>"
// line embedded inside a YAML block scalar (e.g., a code example in an examples: field).
// Before the fix, TrimSpace caused indented inner kind lines to shadow the real kind.
func TestFindMisplacedObjectFiles_EmbeddedKindInExamples(t *testing.T) {
	dir := t.TempDir()

	// Reproduce the false-positive: a policy file whose examples: block contains
	// "  kind: change_journal_entry" (indented). Without the fix, TrimSpace caused
	// the indented inner kind line to shadow the real kind, reporting the file as a
	// misplaced change_journal_entry.
	// NOTE: the indented "kind: change_journal_entry" below simulates what appears
	// inside a code example block in a real policy file.
	policyYAML := "id: POL-CODE-999\n" +
		"kind: policy\n" +
		"title: Test Policy\n" +
		"examples:\n" +
		"    - |\n" +
		"      Incorrect: id: CHA-001\n" +
		"      kind: change_journal_entry\n" +
		"      status: draft\n"
	casDir := datacell.CellCASPrimaryDir(dir, "policies")
	if err := fileutil.EnsureDir(casDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fileName := "abc123.yaml"
	if err := fileutil.WriteSecureFile(filepath.Join(casDir, fileName), []byte(policyYAML)); err != nil {
		t.Fatalf("write: %v", err)
	}

	misplaced, err := storage.FindMisplacedObjectFiles(dir)
	if err != nil {
		t.Fatalf("FindMisplacedObjectFiles: %v", err)
	}

	// The policy file is in policies/, which is the correct directory for kind:policy.
	// It must NOT appear in the misplaced map (as a change_journal_entry false positive).
	if len(misplaced) > 0 {
		for kind, files := range misplaced {
			for _, f := range files {
				t.Errorf("false-positive misplacement reported: kind=%q file=%q (expected 0 misplaced)", kind, f)
			}
		}
	}
}

// TestFindMisplacedObjectFiles_ActualMisplacement checks that a genuinely
// misplaced file (e.g., a backlog_item in the policies/ dir) is still detected.
func TestFindMisplacedObjectFiles_ActualMisplacement(t *testing.T) {
	dir := t.TempDir()

	// A backlog_item placed in a policies/ directory is genuinely misplaced.
	wrongDirYAML := `id: BLI-999
kind: backlog_item
title: Misplaced Item
status: planned
`
	casDir := datacell.CellCASPrimaryDir(dir, "policies")
	if err := fileutil.EnsureDir(casDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(casDir, "misplaced.yaml"), []byte(wrongDirYAML)); err != nil {
		t.Fatalf("write: %v", err)
	}

	misplaced, err := storage.FindMisplacedObjectFiles(dir)
	if err != nil {
		t.Fatalf("FindMisplacedObjectFiles: %v", err)
	}
	if len(misplaced["backlog_item"]) == 0 {
		t.Errorf("expected misplaced[backlog_item] to contain the file, got %v", misplaced)
	}
}
