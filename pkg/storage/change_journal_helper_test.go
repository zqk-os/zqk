package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/paths"
	_ "github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestCreateChangeJournalEntryWithBuilder_LifetimeCounters(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "test-project")
	mustEnsureProcessSpecsLayout(t, tmpDir)
	journalDir := filepath.Join(tmpDir, paths.ProcessDir, "change_journal_entry")
	if err := fileutil.MkdirAll(journalDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	fileStorage, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create FileObjectStorage: %v", err)
	}

	defer func() { _ = fileStorage.Shutdown(context.Background()) }()

	createdBefore, failedBefore := GetChangeJournalHelperStats()

	options := &ChangeJournalEntryOptions{
		ChangeType:  OpUpdate,
		ObjectRef:   "test_kind:TEST-001",
		DiffSummary: "updated status",
		Title:       "Update: test_kind:TEST-001",
		CreatedBy:   "system",
	}

	err = CreateChangeJournalEntryWithBuilder(
		context.Background(),
		tmpDir,
		pkgctx.NewSystemSecurityContext(),
		fileStorage,
		options,
	)
	if err != nil {
		t.Fatalf("CreateChangeJournalEntryWithBuilder failed: %v", err)
	}

	createdAfter, failedAfter := GetChangeJournalHelperStats()
	if createdAfter != createdBefore+1 {
		t.Errorf("expected created to increase by 1, got before=%d after=%d", createdBefore, createdAfter)
	}
	if failedAfter != failedBefore {
		t.Errorf("expected failed to remain %d, got %d", failedBefore, failedAfter)
	}
}
