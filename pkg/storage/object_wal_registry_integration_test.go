package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestObjectWAL_SharedAcquire_CompactDoesNotLeakFDs ensures multiple acquires share one
// ObjectWAL and CompactInPlace reuses the same pointer (no NewObjectWAL replace).
func TestObjectWAL_SharedAcquire_CompactDoesNotLeakFDs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)

	wal1, err := AcquireObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("AcquireObjectWAL: %v", err)
	}
	wal2, err := AcquireObjectWAL(tmpDir)
	if err != nil {
		t.Fatalf("AcquireObjectWAL second: %v", err)
	}
	if wal1 != wal2 {
		t.Fatal("expected shared *ObjectWAL for same project root")
	}
	if got := objectWALRefCountForTest(tmpDir); got != 2 {
		t.Fatalf("refs=%d want 2", got)
	}

	rec := &WALRecord{Op: "delete", Kind: "backlog_item", ID: "BLI-WAL-FD-1"}
	if err := wal1.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := wal1.Sync(); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if err := WriteAppliedSeq(tmpDir, rec.Seq); err != nil {
		t.Fatalf("WriteAppliedSeq: %v", err)
	}

	ptrBefore := wal1
	if err := wal1.CompactInPlace(tmpDir); err != nil {
		t.Fatalf("CompactInPlace: %v", err)
	}
	if wal1 != ptrBefore || wal2 != ptrBefore {
		t.Fatal("CompactInPlace must not replace *ObjectWAL pointer")
	}

	// Still writable on the shared instance after compact.
	rec2 := &WALRecord{Op: "delete", Kind: "backlog_item", ID: "BLI-WAL-FD-2"}
	if err := wal2.Append(rec2); err != nil {
		t.Fatalf("Append after compact: %v", err)
	}

	if err := ReleaseObjectWAL(tmpDir, wal1); err != nil {
		t.Fatalf("ReleaseObjectWAL: %v", err)
	}
	if got := objectWALRefCountForTest(tmpDir); got != 1 {
		t.Fatalf("refs after one release=%d want 1", got)
	}
	if err := ReleaseObjectWAL(tmpDir, wal2); err != nil {
		t.Fatalf("ReleaseObjectWAL second: %v", err)
	}
	if got := objectWALRefCountForTest(tmpDir); got != 0 {
		t.Fatalf("refs after final release=%d want 0", got)
	}

	walPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir, objectWALFileName)
	if _, err := fileutil.Stat(walPath); err != nil {
		t.Fatalf("wal path missing after release: %v", err)
	}
}

// TestTryCompactWAL_ReusesSameWALPointer covers FileObjectStorage compaction path.
func TestTryCompactWAL_ReusesSameWALPointer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Shutdown(context.Background())
	})
	if st.wal == nil {
		t.Fatal("expected WAL")
	}
	before := st.wal
	rec := &WALRecord{Op: "delete", Kind: "backlog_item", ID: "BLI-WAL-FD-3"}
	if err := st.wal.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	_ = st.wal.Sync()
	_ = WriteAppliedSeq(tmpDir, rec.Seq)

	if err := st.TryCompactWAL(); err != nil {
		t.Fatalf("TryCompactWAL: %v", err)
	}
	if st.wal != before {
		t.Fatal("TryCompactWAL replaced *ObjectWAL — leaks FDs when write-behind still holds old pointer")
	}
}
