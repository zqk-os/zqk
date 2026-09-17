package system

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCompactChangeJournalWindow_Integration(t *testing.T) {
	// TRACK: BLI-1785443942668406000-1ec5c811 — no t.Parallel with t.Setenv (Go 1.26+).
	projectRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), projectRoot)
	if _, err := setupSystemTestEnvironmentRoot(t, projectRoot); err != nil {
		t.Fatalf("setupSystemTestEnvironmentRoot: %v", err)
	}

	fileStorage, err := storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, projectRoot, fileStorage)

	compactor := storage.NewChangeJournalCompactionService(fileStorage)
	if compactor == nil {
		t.Fatalf("NewChangeJournalCompactionService returned nil")
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	now := time.Now()
	res, err := compactor.CompactWindow(
		context.Background(),
		secCtx,
		pkgctx.GetStorageContext(),
		now.Add(-1*time.Hour),
		now,
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("CompactWindow returned unexpected error: %v", err)
	}
	if res == nil {
		t.Fatalf("CompactWindow returned nil CompactionResult")
	}
	if res.EntryCount != 0 {
		t.Errorf("CompactWindow EntryCount = %d, want 0 for empty window", res.EntryCount)
	}
}

func TestNewCompactJournalCmd_FlagsAndStructure(t *testing.T) {
	t.Parallel()

	cmd := NewCompactJournalCmd()
	if cmd == nil {
		t.Fatalf("NewCompactJournalCmd returned nil")
	}

	if cmd.Use != "compact-journal" {
		t.Errorf("cmd.Use = %q, want 'compact-journal'", cmd.Use)
	}

	windowFlag := cmd.Flags().Lookup("window-hours")
	if windowFlag == nil {
		t.Fatalf("window-hours flag missing")
	}
	if windowFlag.DefValue != "24" {
		t.Errorf("window-hours default = %q, want '24'", windowFlag.DefValue)
	}

	outputFlag := cmd.Flags().Lookup("output-dir")
	if outputFlag == nil {
		t.Fatalf("output-dir flag missing")
	}
}
