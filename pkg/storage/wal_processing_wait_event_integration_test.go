package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// TestEnsureCLIObjectMutationVisibleForProvider_EventDrivenWait verifies the cross-process
// durability barrier: enqueue write-behind ops in one storage instance, then use
// EnsureCLIObjectMutationVisibleForProvider with a non-file provider (nil) to wait for
// WAL checkpoint progress using event notifications. Finally, verify the object is readable
// from disk using a fresh storage instance without write-behind.
func TestEnsureCLIObjectMutationVisibleForProvider_EventDrivenWait(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tmpDir)
	t.Cleanup(func() {
		if err := RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil)); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})

	setupTestRootLikeSetupTestEnvironmentWithSpecsOrSkip(t, tmpDir)

	// Writer: write-behind enabled.
	writer, err := NewFileObjectStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorage (writer): %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = writer.Shutdown(shutdownCtx)
	})

	// Reader: write-behind disabled; read must come from disk.
	reader, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest (reader): %v", err)
	}
	defer func() { _ = reader.Shutdown(context.Background()) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = reader.Shutdown(shutdownCtx)
	}()

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	id := "ITEM-WB-EVENT-001"
	createCtx := pkgctx.WithCacheUpdate(ctx, id, "backlog_item", "")
	obj := minimalBacklogItemForWriteBehind(id)

	if err := writer.Create(createCtx, secCtx, obj); err != nil {
		t.Fatalf("Create (writer): %v", err)
	}

	// Best-effort pre-check: object may or may not be visible yet depending on timing.
	_, preErr := reader.Read(ctx, secCtx, id)
	_ = preErr // don't fail: we're testing the durability barrier below

	flushCtx, cancel := DurabilityFlushContext()
	defer cancel()

	// Use nil provider to force cross-process fallback path (non-*FileObjectStorage).
	if err := EnsureCLIObjectMutationVisibleForProvider(flushCtx, nil, tmpDir, []string{"backlog_item"}); err != nil {
		t.Fatalf("EnsureCLIObjectMutationVisibleForProvider: %v", err)
	}

	got, err := reader.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read after Ensure: %v", err)
	}
	if got[objects.FieldKeyID] != id {
		t.Fatalf("Read after Ensure: got id=%v want=%v", got[objects.FieldKeyID], id)
	}
}
