package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCreateHealthCheckChangeJournalEntry_LifetimeCounters(t *testing.T) {
	tmpDir := t.TempDir()
	journalDir := filepath.Join(tmpDir, paths.ProcessDir, "change_journal_entry")
	if err := fileutil.MkdirAll(journalDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	testStorage := &TestStorage{objects: make(map[string]map[string]any)}

	createdBefore, failedBefore := GetHealthCheckJournalStats()

	err := CreateHealthCheckChangeJournalEntry(
		context.Background(),
		tmpDir,
		testStorage,
		"MON-001",
		"healthy",
		"All checks passed",
		map[string]any{"cpu": 10},
	)
	if err != nil {
		t.Fatalf("CreateHealthCheckChangeJournalEntry failed: %v", err)
	}

	createdAfter, failedAfter := GetHealthCheckJournalStats()
	if createdAfter != createdBefore+1 {
		t.Errorf("expected created to increase by 1, got before=%d after=%d", createdBefore, createdAfter)
	}
	if failedAfter != failedBefore {
		t.Errorf("expected failed to remain %d, got %d", failedBefore, failedAfter)
	}
}
