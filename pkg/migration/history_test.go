package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestLoadHistory_NoFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	h, err := LoadHistory(root)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if h == nil || len(h.Entries) != 0 {
		t.Errorf("expected empty history, got %+v", h)
	}
}

func TestHasRun_NoHistory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ok, err := HasRun(root, "any-id")
	if err != nil {
		t.Fatalf("HasRun: %v", err)
	}
	if ok {
		t.Error("expected false when no history")
	}
}

func TestRecordSuccess_AndLoadHistory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stateDir := filepath.Join(root, paths.ProjectDataDir, paths.StateDir)
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := RecordSuccess(root, "mig-1"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	h, err := LoadHistory(root)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	if len(h.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(h.Entries))
	}
	if h.Entries[0].MigrationID != "mig-1" {
		t.Errorf("entry migration_id: got %q", h.Entries[0].MigrationID)
	}
	if h.Entries[0].CompletedAt.IsZero() {
		t.Error("expected CompletedAt set")
	}
}

func TestRecordSuccess_CreatesStateDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Do not create state dir beforehand
	if err := RecordSuccess(root, "mig-2"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	p := historyFilePath(root)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("history file not created: %v", err)
	}
}

func TestHasRun_AfterRecordSuccess(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := RecordSuccess(root, "mig-3"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	ok, err := HasRun(root, "mig-3")
	if err != nil {
		t.Fatalf("HasRun: %v", err)
	}
	if !ok {
		t.Error("expected true for recorded migration")
	}
	ok, err = HasRun(root, "other")
	if err != nil {
		t.Fatalf("HasRun other: %v", err)
	}
	if ok {
		t.Error("expected false for unrecorded migration")
	}
}

func TestRecordSuccess_RejectsEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := RecordSuccess("", "id"); err == nil {
		t.Error("expected error for empty project root")
	}
	if err := RecordSuccess(root, ""); err == nil {
		t.Error("expected error for empty migration id")
	}
}
