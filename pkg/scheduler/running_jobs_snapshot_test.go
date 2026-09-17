package scheduler

import (
	"path/filepath"
	"testing"
)

func TestPersistLoadRunningJobsSnapshot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := PersistRunningJobsSnapshot(root, []string{"SCH-run-bundle-2", "SCH-run-bundle-1"}); err != nil {
		t.Fatalf("PersistRunningJobsSnapshot: %v", err)
	}
	path := RunningJobsSnapshotPath(root)
	if filepath.Base(path) != runningJobsSnapshotFile {
		t.Fatalf("path base=%q", filepath.Base(path))
	}
	ids, updated, ok := LoadRunningJobsSnapshot(root)
	if !ok {
		t.Fatal("expected ok")
	}
	if updated.IsZero() {
		t.Fatal("expected updated_at")
	}
	if len(ids) != 2 || ids[0] != "SCH-run-bundle-1" || ids[1] != "SCH-run-bundle-2" {
		t.Fatalf("ids=%v (want sorted)", ids)
	}
	if err := PersistRunningJobsSnapshot(root, nil); err != nil {
		t.Fatalf("persist empty: %v", err)
	}
	ids, _, ok = LoadRunningJobsSnapshot(root)
	if !ok || len(ids) != 0 {
		t.Fatalf("empty snapshot: ok=%v ids=%v", ok, ids)
	}
}
