package storage

import (
	"context"

	"github.com/lanceman/zqk/pkg/datacell"

	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestChangeJournalCompactionService_CompactWindow(t *testing.T) {

	// Setup
	dir := t.TempDir()

	// Create required process directory
	processDir := datacell.ProcessPrimaryDir(dir)
	if err := os.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// Create specs dir and spec
	specsDir := filepath.Join(processDir, "_internal", "object_specs")
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	specContent := `
kind: change_journal_entry
id_prefix: CHA
fields:
  id: {type: string, required: true}
  kind: {type: string, required: true}
  object_ref: {type: string, required: true}
  change_type: {type: string, required: true}
  created_at: {type: datetime, required: true}
  created_by: {type: string, required: true}
  changed_paths: {type: list, required: false}
`
	if err := os.WriteFile(filepath.Join(specsDir, "change_journal_entry.yaml"), []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	// Create lifecycles dir (storage requires it)
	lifecyclesDir := filepath.Join(processDir, "_internal", "lifecycles")
	if err := os.MkdirAll(lifecyclesDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// Path cache required for stream/CAS resolution (change_journal_helper, GetStreamSegmentDir)
	BuildPathAliasCacheForProject(dir)

	// Use NewFileObjectStorageForTest to disable async WAL/write-behind
	storage, err := NewFileObjectStorageForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = storage.Shutdown(context.Background()) }()

	// Ensure cleanup (drain hash registries)
	t.Cleanup(func() {
		if cleanup := storage.GetTestCleanup(); cleanup != nil {
			cleanup()
		}
	})

	service := NewChangeJournalCompactionService(storage)
	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	// Create some entries in the window
	windowStart := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2030, 2, 1, 23, 59, 59, 0, time.UTC)

	entries := []map[string]any{
		{
			objects.FieldKeyID:           "CHA-1",
			objects.FieldKeyKind:         "change_journal_entry",
			objects.FieldKeyObjectRef:    "backlog_item:ITEM-001",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyCreatedAt:    "2030-02-01T10:00:00Z",
			objects.FieldKeyCreatedBy:    "account:system",
			objects.FieldKeyChangedPaths: []any{"status"},
		},
		{
			objects.FieldKeyID:           "CHA-2",
			objects.FieldKeyKind:         "change_journal_entry",
			objects.FieldKeyObjectRef:    "backlog_item:ITEM-002",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyCreatedAt:    "2030-02-01T11:00:00Z",
			objects.FieldKeyCreatedBy:    "account:system",
			objects.FieldKeyChangedPaths: []any{"title"},
		},
		{
			objects.FieldKeyID:           "CHA-3", // Outside window
			objects.FieldKeyKind:         "change_journal_entry",
			objects.FieldKeyObjectRef:    "backlog_item:ITEM-003",
			objects.FieldKeyChangeType:   "update",
			objects.FieldKeyCreatedAt:    "2030-02-02T10:00:00Z",
			objects.FieldKeyCreatedBy:    "account:system",
			objects.FieldKeyChangedPaths: []any{"status"},
		},
	}

	res, err := storage.BulkCreate(ctx, secCtx, entries)
	if err != nil {
		t.Fatalf("BulkCreate failed: %v", err)
	}
	if res.FailureCount > 0 {
		t.Fatalf("BulkCreate partial failure: %v", res.Errors)
	}

	// Flush CAS queue to ensure index is updated
	casQueue := GetListingIndexWriteQueueForProjectRoot(dir)
	if err := casQueue.FlushKind("change_journal_entry", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush CAS queue: %v", err)
	}

	// Define output dir
	outputDir := filepath.Join(dir, "archive", "change_journal")

	// Run compaction
	result, err := service.CompactWindow(ctx, secCtx, storageCtx, windowStart, windowEnd, outputDir)
	if err != nil {
		t.Fatalf("CompactWindow failed: %v", err)
	}

	// Verify result
	if result.EntryCount != 2 {
		t.Errorf("EntryCount = %d, want 2", result.EntryCount)
	}
	// Note: CompressEntryIDs only compresses ranges of 3 or more. For 2 items, it lists them individually.
	if len(result.EntryIDRanges) != 2 || result.EntryIDRanges[0] != "CHA-1" || result.EntryIDRanges[1] != "CHA-2" {
		t.Errorf("EntryIDRanges = %v, want [CHA-1, CHA-2]", result.EntryIDRanges)
	}

	// Verify artifact exists
	if _, err := os.Stat(result.ArtifactPath); os.IsNotExist(err) {
		t.Errorf("Artifact not created at %s", result.ArtifactPath)
	}

	// Flush CAS queue again before checking exists (deletes might be queued)
	if err := casQueue.FlushKind("change_journal_entry", 5*time.Second); err != nil {
		t.Fatalf("Failed to flush CAS queue after delete: %v", err)
	}

	// Verify originals deleted
	exists1, _ := storage.Exists(ctx, secCtx, "CHA-1")
	if exists1 {
		t.Error("CHA-1 should have been deleted")
	}
	exists2, _ := storage.Exists(ctx, secCtx, "CHA-2")
	if exists2 {
		t.Error("CHA-2 should have been deleted")
	}

	// Verify outside window entry remains
	exists3, _ := storage.Exists(ctx, secCtx, "CHA-3")
	if !exists3 {
		t.Error("CHA-3 should still exist")
	}

	compactions, entriesCompacted := service.GetCompactionStats()
	if compactions != 1 {
		t.Errorf("GetCompactionStats compactions = %d, want 1", compactions)
	}
	if entriesCompacted != 2 {
		t.Errorf("GetCompactionStats entriesCompacted = %d, want 2", entriesCompacted)
	}
}
