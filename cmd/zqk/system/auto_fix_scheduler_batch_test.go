package system

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAutoFixSchedulerBatcher_CreateBatches_GroupsByObject(t *testing.T) {
	t.Parallel()

	b := &AutoFixSchedulerBatcher{projectRoot: "/tmp", batchSize: 100}
	issues := []AutoFixBatchIssue{
		{ObjectID: "CHA-1", ObjectKind: "change_journal_entry", FilePath: "p1", Issue: Issue{Category: "instance_validation", Message: "created_at: bad", AutoFixable: true}},
		{ObjectID: "CHA-1", ObjectKind: "change_journal_entry", FilePath: "p1", Issue: Issue{Category: "instance_validation", Message: "updated_at: bad", AutoFixable: true}},
		{ObjectID: "CHA-2", ObjectKind: "change_journal_entry", FilePath: "p2", Issue: Issue{Category: "integrity", Message: "hash missing", AutoFixable: true}},
	}

	batches := b.CreateBatches(issues)
	if len(batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(batches))
	}
	batch := batches[0]
	if batch.Progress.Total != 3 {
		t.Fatalf("expected total=3, got %d", batch.Progress.Total)
	}
	if len(batch.Issues) != 0 {
		t.Fatalf("expected legacy Issues empty in new format, got %d", len(batch.Issues))
	}
	if len(batch.Objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(batch.Objects))
	}

	// CHA-1 should have 2 issues.
	var cha1 *AutoFixBatchObject
	for i := range batch.Objects {
		if batch.Objects[i].ObjectID == "CHA-1" {
			cha1 = &batch.Objects[i]
		}
	}
	if cha1 == nil {
		t.Fatalf("expected CHA-1 object entry")
	}
	if len(cha1.Issues) != 2 {
		t.Fatalf("expected CHA-1 issues=2, got %d", len(cha1.Issues))
	}
}

func TestRemoveUnsubmittedAutoFixBatchFile(t *testing.T) {
	t.Parallel()

	t.Run("removes orphan when submission did not persist", func(t *testing.T) {
		batchFile := filepath.Join(t.TempDir(), "AUTOFIX-orphan.json")
		if err := fileutil.WriteFile(batchFile, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := removeUnsubmittedAutoFixBatchFile(batchFile, false); err != nil {
			t.Fatalf("removeUnsubmittedAutoFixBatchFile() error = %v", err)
		}
		if _, err := fileutil.Stat(batchFile); !fileutil.IsNotExist(err) {
			t.Fatalf("orphan batch still exists or stat returned unexpected error: %v", err)
		}
	})

	t.Run("retains batch after scheduler persistence", func(t *testing.T) {
		batchFile := filepath.Join(t.TempDir(), "AUTOFIX-submitted.json")
		if err := fileutil.WriteFile(batchFile, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := removeUnsubmittedAutoFixBatchFile(batchFile, true); err != nil {
			t.Fatalf("removeUnsubmittedAutoFixBatchFile() error = %v", err)
		}
		if _, err := fileutil.Stat(batchFile); err != nil {
			t.Fatalf("submitted batch was removed: %v", err)
		}
	})
}
