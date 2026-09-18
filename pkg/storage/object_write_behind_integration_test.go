// Integration tests for write-behind (Option B): storage created with write-behind enabled
// (real project root, no SkipGlobalWiring). Validates create -> read from pending, drain ->
// read from disk, delete -> not found. Most tests use NewFileObjectStorageForTest and do not
// enable write-behind; this file explicitly enables it to prevent regression.

package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/zqk-os/zqk/pkg/zqktime"
)

// minimalBacklogItemForWriteBehind returns a valid backlog_item for write-behind integration test.
func minimalBacklogItemForWriteBehind(id string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	return map[string]any{
		objects.FieldKeyID: id, objects.FieldKeyKind: "backlog_item", objects.FieldKeyTitle: "Write-behind test",
		objects.FieldKeyStatus: objects.ObjectStatusExploring, objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt: now, objects.FieldKeyCreatedBy: "ACC-1785920548450214012-68b850c0", objects.FieldKeyUpdatedAt: now, objects.FieldKeyUpdatedBy: "ACC-1785920548450214012-68b850c0",
	}
}

// TestWriteBehind_CreateReadDrainReadDelete creates storage with write-behind enabled,
// creates an object, reads (pending), drains, reads again (disk), deletes, reads (not found).
func TestWriteBehind_CreateReadDrainReadDelete(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Storage with write-behind enabled (no SkipGlobalWiring)
	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}

	if st.writeBuf == nil || st.wal == nil || st.writeBehindWorker == nil {
		t.Fatal("write-behind not enabled (writeBuf/wal/worker nil)")
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()
	createCtx := pkgctx.WithCacheUpdate(ctx, "BLI-WB-001", "backlog_item", "")
	id := "BLI-WB-001"
	obj := minimalBacklogItemForWriteBehind(id)

	// Create (enqueued to WAL+buffer, returns immediately)
	if err := st.Create(createCtx, secCtx, obj); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read must see object from pending
	got, err := st.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read after create: %v", err)
	}
	if got[objects.FieldKeyID] != id || got[objects.FieldKeyKind] != "backlog_item" {
		t.Errorf("Read: got %v", got)
	}

	// Drain and close
	shutdownCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := st.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// New storage (same root), read from disk
	st2, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage (second): %v", err)
	}

	defer func() {
		ctx2, c2 := context.WithTimeout(context.Background(), 5*time.Second)
		defer c2()
		_ = st2.Shutdown(ctx2)
	}()

	got2, err := st2.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read after drain (from disk): %v", err)
	}
	if got2[objects.FieldKeyID] != id {
		t.Errorf("Read from disk: got %v", got2)
	}

	// Delete (requires CLI context)
	cliCtx := WithTestHardDelete(ctx)
	if err := st2.Delete(cliCtx, secCtx, id, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = st2.Read(ctx, secCtx, id)
	if !errors.Is(err, ErrObjectNotFound) {
		t.Errorf("Read after delete: want ErrObjectNotFound, got %v", err)
	}
}

// TestWriteBehind_DisabledWhenSkipGlobalWiring ensures NewFileObjectStorageForTest does not enable write-behind.
func TestWriteBehind_DisabledWhenSkipGlobalWiring(t *testing.T) {
	tmpDir := t.TempDir()
	SetupTestRootLikeSetupTestEnvironmentForExportTest(t, tmpDir)
	st, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	defer func() { _ = st.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, st)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
	if st.writeBuf != nil || st.wal != nil || st.writeBehindWorker != nil {
		t.Error("write-behind should be disabled when using NewFileObjectStorageForTest")
	}
}

// TestWriteBehind_DisabledWhenPrivilegedWriterDaemon ensures object daemon does not
// claim write-behind or replay object.wal (8GB RSS / EMFILE footgun).
// TRACK: BLI-CEF-R20-SINGLE-WRITER-BLI-001
func TestWriteBehind_DisabledWhenPrivilegedWriterDaemonArgvWithoutEnv(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Setenv(zqkenv.IsDaemon().Name(), "")
	setPrivilegedWriterCommandArgs(t, []string{"zqk-stable", "object", "daemon"})
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	defer func() { _ = st.Shutdown(context.Background()) }()
	if st.writeBuf != nil || st.wal != nil || st.writeBehindWorker != nil {
		t.Error("write-behind must stay off from object daemon argv even when IS_DAEMON is unset")
	}
}

func TestWriteBehind_DisabledWhenPrivilegedWriterDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Setenv(zqkenv.IsDaemon().Name(), "1")
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	st, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	defer func() { _ = st.Shutdown(context.Background()) }()
	if st.writeBuf != nil || st.wal != nil || st.writeBehindWorker != nil {
		t.Error("write-behind must stay off in the privileged-writer daemon process")
	}
}
