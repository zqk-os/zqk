package cas

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestExtractRefFields_collectsAllRefTypes(t *testing.T) {
	t.Parallel()
	obj := map[string]any{
		objects.FieldKeyID:              "BLI-001",
		objects.FieldKeyKind:            "backlog_item",
		objects.FieldKeyTitle:           "Test item",
		"parent_ref":                    "REQ-001",
		objects.FieldKeyGoalRefs:        []any{"GOL-001", "GOL-002"},
		objects.FieldKeyPriorityPlanRef: "PRI-001",
		objects.FieldKeyDecisionRefs:    []any{"DEC-001"},
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
	}

	refs := extractRefFields(obj)
	if len(refs) == 0 {
		t.Fatal("expected ref fields to be extracted, got none")
	}

	expected := map[string]bool{
		"REQ-001": true,
		"GOL-001": true,
		"GOL-002": true,
		"PRI-001": true,
		"DEC-001": true,
	}
	for _, r := range refs {
		if !expected[r] {
			t.Errorf("unexpected ref: %s", r)
		}
		delete(expected, r)
	}
	if len(expected) > 0 {
		t.Errorf("missing refs: %v", expected)
	}
}

func TestCASOrphanGuard_CheckPackageIntegrity_detectsOrphans(t *testing.T) {
	t.Parallel()
	// Create a temp repo structure with an orphan reference.
	dir := t.TempDir()
	processDir := filepath.Join(dir, "docs", "process", "backlog")
	if err := fileutil.MkdirAll(processDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Object A: references B
	objA := `id: BLI-ALPHA
kind: backlog_item
title: Alpha
goal_refs:
  - GOL-MISSING
`
	if err := fileutil.WriteFile(filepath.Join(processDir, "alpha.yaml"), []byte(objA), 0644); err != nil {
		t.Fatal(err)
	}

	// Object B does NOT exist — GOL-MISSING is an orphan reference.

	guard := NewCASOrphanGuard(dir)
	results, err := guard.CheckPackageIntegrity()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected orphan detection for GOL-MISSING, got none")
	}

	found := false
	for _, r := range results {
		if r.DeletedID == "GOL-MISSING" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected GOL-MISSING in results, got: %v", results)
	}
}

func TestCASOrphanGuard_CheckPackageIntegrity_cleanTree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	processDir := filepath.Join(dir, "docs", "process", "goals")
	if err := fileutil.MkdirAll(processDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a self-consistent pair: BLI references GOL, GOL exists.
	goalYAML := `id: GOL-001
kind: goal
title: Test Goal
`
	bliDir := filepath.Join(dir, "docs", "process", "backlog")
	if err := fileutil.MkdirAll(bliDir, 0755); err != nil {
		t.Fatal(err)
	}
	bliYAML := `id: BLI-001
kind: backlog_item
title: Test BLI
goal_refs:
  - GOL-001
`
	if err := fileutil.WriteFile(filepath.Join(processDir, "goal.yaml"), []byte(goalYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(bliDir, "bli.yaml"), []byte(bliYAML), 0644); err != nil {
		t.Fatal(err)
	}

	guard := NewCASOrphanGuard(dir)
	results, err := guard.CheckPackageIntegrity()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected no orphans in clean tree, got %d: %v", len(results), results)
	}
}

func TestCASOrphanGuard_CheckPackageIntegrity_skipsInternalDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	internalDir := filepath.Join(dir, "docs", "process", "_internal")
	if err := fileutil.MkdirAll(internalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Internal file with a dangling ref should be ignored.
	internal := `id: INTERNAL-001
kind: schema
parent_ref: DOES-NOT-EXIST
`
	if err := fileutil.WriteFile(filepath.Join(internalDir, "schema.yaml"), []byte(internal), 0644); err != nil {
		t.Fatal(err)
	}

	guard := NewCASOrphanGuard(dir)
	results, err := guard.CheckPackageIntegrity()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected _internal to be skipped, got %d results", len(results))
	}
}
