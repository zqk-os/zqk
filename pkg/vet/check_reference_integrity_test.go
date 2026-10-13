package vet

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckReferenceIntegrity(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	bliDir := filepath.Join(tmpDir, ".zqk", "process", "backlog_items")
	if err := fileutil.MkdirAll(bliDir, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()

	// 1. Valid item with goal_refs
	validWithGoal := `kind: backlog_item
id: BLI-001
status: planned
goal_refs:
  - GOAL-001
`
	if err := fileutil.WriteFile(filepath.Join(bliDir, "bli-1.yaml"), []byte(validWithGoal), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Valid item with requirement_refs
	validWithReq := `kind: backlog_item
id: BLI-002
status: in_progress
requirement_refs:
  - REQ-001
`
	if err := fileutil.WriteFile(filepath.Join(bliDir, "bli-2.yaml"), []byte(validWithReq), 0644); err != nil {
		t.Fatal(err)
	}

	// 3. Preliminary/draft item without refs (should be skipped)
	draftItem := `kind: backlog_item
id: BLI-003
status: draft
`
	if err := fileutil.WriteFile(filepath.Join(bliDir, "bli-3.yaml"), []byte(draftItem), 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Missing goal and req refs on planned item -> SeverityError
	invalidItem := `kind: backlog_item
id: BLI-004
status: planned
`
	if err := fileutil.WriteFile(filepath.Join(bliDir, "bli-4.yaml"), []byte(invalidItem), 0644); err != nil {
		t.Fatal(err)
	}

	findings, err := CheckReferenceIntegrity(tmpDir, cfg)
	if err != nil {
		t.Fatalf("CheckReferenceIntegrity returned unexpected error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %+v", len(findings), findings)
	}

	f := findings[0]
	if f.Severity != SeverityError {
		t.Errorf("expected SeverityError, got %s", f.Severity)
	}
	if f.CheckID != "reference/missing-goal-req-ref" {
		t.Errorf("expected check ID 'reference/missing-goal-req-ref', got %s", f.CheckID)
	}
	if f.Suite != "hygiene" {
		t.Errorf("expected suite 'hygiene', got %s", f.Suite)
	}
}
