package system

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/storage"
)

func TestCompactChangeJournalWindow_Integration(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", projectRoot)

	factory, err := storage.NewStorageFactory(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}

	compactor := storage.NewChangeJournalCompactionService(factory.GetStorage())
	if compactor == nil {
		t.Fatalf("NewChangeJournalCompactionService returned nil")
	}

	now := time.Now()
	res, err := compactor.CompactWindow(
		context.Background(),
		nil,
		nil,
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
