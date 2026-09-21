package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/zqktime"
)

// TestObjectWriteBehindWorker_SecurityContext verifies that the write-behind worker
// uses a non-nil system security context, preventing nil pointer dereferences when
// applying operations that require permission checks.
func TestObjectWriteBehindWorker_SecurityContext(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Create storage with write-behind enabled
	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	if st.writeBuf == nil || st.wal == nil || st.writeBehindWorker == nil {
		t.Fatal("write-behind not enabled (writeBuf/wal/worker nil)")
	}
	t.Cleanup(func() { _ = st.Shutdown(context.Background()) })

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create an object that will trigger a read operation during update
	// (update operations read the old object to update reverse references)
	id := "BLI-SEC-CTX-001"
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Security context test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}

	// Create object (enqueued to write-behind buffer)
	createCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")
	if err := st.Create(createCtx, secCtx, obj); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Update the object - this will trigger applyUpdateFromBuffer which calls Read
	// with the worker's security context. If the context is nil, this will panic.
	updateCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")
	updates := map[string]any{objects.FieldKeyTitle: "Updated title"}
	if err := st.Update(updateCtx, secCtx, id, updates); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Wait for write-behind worker to process the update
	var got map[string]any
	ok := concurrency.PollTimeout(2*time.Second, 10*time.Millisecond, func() bool {
		var readErr error
		got, readErr = st.Read(ctx, secCtx, id)
		return readErr == nil && got[objects.FieldKeyTitle] == "Updated title"
	})
	if !ok {
		t.Fatalf("Update not applied within timeout: got title %v, want 'Updated title'", got[objects.FieldKeyTitle])
	}

	// Shutdown to ensure all operations complete
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := st.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

// TestObjectWriteBehindWorker_ApplyOperationsWithSystemContext verifies that
// apply operations (create, update, delete) work correctly with system security context.
func TestObjectWriteBehindWorker_ApplyOperationsWithSystemContext(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Create storage with write-behind enabled
	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = st.Shutdown(shutdownCtx)
	}()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Test create operation
	id1 := "BLI-SEC-CTX-002"
	obj1 := map[string]any{
		objects.FieldKeyID:            id1,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Create test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createCtx := pkgctx.WithCacheUpdate(ctx, id1, "backlog_item", "")
	if err := st.Create(createCtx, secCtx, obj1); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Test update operation (requires read of old object)
	id2 := "BLI-SEC-CTX-003"
	obj2 := map[string]any{
		objects.FieldKeyID:            id2,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Update test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createCtx2 := pkgctx.WithCacheUpdate(ctx, id2, "backlog_item", "")
	if err := st.Create(createCtx2, secCtx, obj2); err != nil {
		t.Fatalf("Create for update test: %v", err)
	}

	// Update (this triggers applyUpdateFromBuffer -> Read with system context)
	updateCtx := pkgctx.WithCacheUpdate(ctx, id2, "backlog_item", "")
	updates := map[string]any{objects.FieldKeyTitle: "Updated"}
	if err := st.Update(updateCtx, secCtx, id2, updates); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Test delete operation
	id3 := "BLI-SEC-CTX-004"
	obj3 := map[string]any{
		objects.FieldKeyID:            id3,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Delete test",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
		objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
	}
	createCtx3 := pkgctx.WithCacheUpdate(ctx, id3, "backlog_item", "")
	if err := st.Create(createCtx3, secCtx, obj3); err != nil {
		t.Fatalf("Create for delete test: %v", err)
	}

	// Delete (requires CLI operation context)
	deleteCtx := WithTestHardDelete(ctx)
	if err := st.Delete(deleteCtx, secCtx, id3, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Poll until all operations complete
	var got1, got2 map[string]any
	var err3 error
	ok := concurrency.PollTimeout(2*time.Second, 10*time.Millisecond, func() bool {
		var err1, err2 error
		got1, err1 = st.Read(ctx, secCtx, id1)
		got2, err2 = st.Read(ctx, secCtx, id2)
		_, err3 = st.Read(ctx, secCtx, id3)
		return err1 == nil && got1[objects.FieldKeyTitle] == "Create test" &&
			err2 == nil && got2[objects.FieldKeyTitle] == "Updated" &&
			errors.Is(err3, ErrObjectNotFound)
	})
	if !ok {
		t.Fatalf("Operations not completed within timeout: got1=%v, got2=%v, err3=%v", got1, got2, err3)
	}
}

// TestObjectWriteBehindWorker_NoNilPointerDereference verifies that operations
// don't panic with nil pointer dereference when security context is properly set.
// This test specifically targets the bug where secCtx was nil in Start() and run().
func TestObjectWriteBehindWorker_NoNilPointerDereference(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Create storage with write-behind enabled
	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = st.Shutdown(shutdownCtx)
	}()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Create multiple objects to trigger write-behind processing
	// Update operations will trigger Read calls that require permission checks
	for i := 0; i < 5; i++ {
		id := "BLI-SEC-CTX-005-" + string(rune('A'+i))
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         "Test object",
			objects.FieldKeyStatus:        objects.ObjectStatusExploring,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyCreatedBy:     "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
			objects.FieldKeyUpdatedBy:     "ACC-SYSTEM",
		}
		createCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")
		if err := st.Create(createCtx, secCtx, obj); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}

		// Wait a bit, then update (triggers Read in applyUpdateFromBuffer)
		time.Sleep(50 * time.Millisecond)
		updateCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")
		updates := map[string]any{objects.FieldKeyTitle: "Updated"}
		if err := st.Update(updateCtx, secCtx, id, updates); err != nil {
			t.Fatalf("Update %s: %v", id, err)
		}
	}

	// Poll until all updates are applied
	ok := concurrency.PollTimeout(3*time.Second, 10*time.Millisecond, func() bool {
		for i := 0; i < 5; i++ {
			id := "BLI-SEC-CTX-005-" + string(rune('A'+i))
			got, err := st.Read(ctx, secCtx, id)
			if err != nil || got[objects.FieldKeyTitle] != "Updated" {
				return false
			}
		}
		return true
	})
	if !ok {
		t.Fatal("Bulk updates not applied within timeout")
	}
}

// TestObjectWriteBehindWorker_PostStartupCompaction guards the fix for the WAL replay
// overhead bug identified via profiling samples (2026-03-19):
//
// Without the fix, all 10 write-behind workers independently read and JSON-parsed the
// entire WAL file every 250ms even when all entries were already applied, producing
// ~190,000 wasted JSON parses/second. The fix unconditionally compacts the WAL after
// startup replay completes so each process run begins with an empty WAL.
//
// This test verifies that after all WAL entries have been applied (startup replay
// completes), the WAL file is compacted to empty before the main run loop starts.
func TestObjectWriteBehindWorker_PostStartupCompaction(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Phase 1: create storage, write some objects so WAL accumulates entries,
	// then shut down before the worker fully applies them. We do this by writing
	// directly to the WAL (bypassing the worker) to simulate accumulated state.
	walPath := filepath.Join(tmpDir, paths.ProjectDataDir, paths.WalDir, "object.wal")
	walDir := filepath.Dir(walPath)
	if err := fileutil.MkdirAll(walDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir WAL dir: %v", err)
	}

	// Write two synthetic WAL records to simulate accumulated state from prior runs.
	// These are v1 format (JSON) which ReplayWALChunk supports. Both seq=1 and seq=2
	// will be older than the checkpoint (appliedSeq=2) we write, so replay produces 0
	// entries, which means startup replay exits immediately and post-startup compaction runs.
	syntheticWAL := `{"op":"delete","kind":"backlog_item","id":"BLI-COMPACT-STALE-001","seq":1}
{"op":"delete","kind":"backlog_item","id":"BLI-COMPACT-STALE-002","seq":2}
`
	if err := fileutil.WriteFile(walPath, []byte(syntheticWAL), paths.FilePerm600); err != nil {
		t.Fatalf("write WAL: %v", err)
	}

	// Write checkpoint indicating seq=2 is already applied.
	ckPath := walPath + ".checkpoint"
	if err := WriteAppliedSeq(tmpDir, 2); err != nil {
		t.Fatalf("WriteAppliedSeq: %v", err)
	}
	_ = ckPath

	// Confirm WAL is non-empty before storage opens.
	info, err := fileutil.Stat(walPath)
	if err != nil {
		t.Fatalf("stat WAL before: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("WAL should be non-empty before storage opens")
	}

	// Phase 2: open storage (triggers Start(), which runs runReplayThenRun).
	// Replay finds 0 unapplied entries (both seq ≤ appliedSeq=2) and exits immediately.
	// Post-startup compaction then clears the WAL.
	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = st.Shutdown(shutdownCtx)
	}()

	// Give the replay goroutine time to complete and compact.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		info, err = fileutil.Stat(walPath)
		if err != nil {
			if fileutil.IsNotExist(err) {
				// Compacted WAL may be recreated empty by next append; treat as 0 size.
				break
			}
			t.Fatalf("stat WAL: %v", err)
		}
		if info.Size() == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The WAL must be empty (or not exist) after post-startup compaction.
	info2, err2 := fileutil.Stat(walPath)
	if err2 != nil && !fileutil.IsNotExist(err2) {
		t.Fatalf("stat WAL after: %v", err2)
	}
	walSize := int64(0)
	if err2 == nil {
		walSize = info2.Size()
	}
	if walSize != 0 {
		t.Errorf("WAL should be empty after post-startup compaction; got %d bytes (regression: workers would replay stale entries every 250ms)", walSize)
	}
}
