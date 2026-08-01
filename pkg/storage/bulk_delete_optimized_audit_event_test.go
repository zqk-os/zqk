// Isolated test for BulkDeleteOptimized on stream-backed audit_events.
// Uses test-scenario-style setup (isolated test root, real data) to verify bulk delete
// succeeds for stream-backed audit_events so retention can reduce count to target.
package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

func setupBulkDeleteStreamTest(t *testing.T) (projectRoot string, storage2 **storage.FileObjectStorage) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}
	var p2 *storage.FileObjectStorage
	storage2 = &p2
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if p2 != nil {
			_ = p2.Shutdown(ctx)
		}
	})
	return tmpDir, storage2
}

// TestBulkDeleteOptimized_StreamBackedAuditEvents verifies that BulkDeleteOptimized
// deletes stream-backed audit_events successfully. Data is created via AppendToStream +
// AppendStreamLocationToRegistry (same as daemon), then a second storage instance
// (simulating CLI) runs BulkDeleteOptimized. SuccessCount must equal len(ids).
func TestBulkDeleteOptimized_StreamBackedAuditEvents(t *testing.T) {

	projectRoot, storage2Ptr := setupBulkDeleteStreamTest(t)
	if err := storage.EnsurePathAliasCacheReady(projectRoot); err != nil {
		t.Fatalf("EnsurePathAliasCacheReady: %v", err)
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-03-01T12:00:00Z")

	const N = 15
	var ids []string
	for i := 1; i <= N; i++ {
		id := "AUD-BULK-" + strconv.Itoa(i)
		ids = append(ids, id)
		obj := map[string]any{
			objects.FieldKeyID:        id,
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyEventType: "object_creation",
			objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
		}
		seg, offset, err := storage.AppendToStream(projectRoot, "audit_event", id, obj, createdAt)
		if err != nil {
			t.Fatalf("AppendToStream %s: %v", id, err)
		}
		loc := storage.FormatStreamLocation(seg, offset)
		if err := storage.AppendStreamLocationToRegistry(projectRoot, "audit_event", id, loc); err != nil {
			t.Fatalf("AppendStreamLocationToRegistry %s: %v", id, err)
		}
	}

	var err error
	*storage2Ptr, err = storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	storage2 := *storage2Ptr

	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	res, err := storage2.BulkDeleteOptimized(ctx, secCtx, ids, false, 4)
	if err != nil {
		t.Fatalf("BulkDeleteOptimized: %v", err)
	}
	if res.SuccessCount != N {
		t.Errorf("BulkDeleteOptimized: SuccessCount = %d, want %d (FailureCount=%d, Errors=%d)",
			res.SuccessCount, N, res.FailureCount, len(res.Errors))
		for _, e := range res.Errors {
			t.Logf("  error: id=%s %v", e.ID, e.Error)
		}
	}

	filter := storage.ListFilter{Kind: "audit_event"}
	count, err := storage2.Count(ctx, secCtx, filter)
	if err != nil {
		t.Fatalf("Count after bulk delete: %v", err)
	}
	if count != 0 {
		t.Errorf("Count after bulk delete = %d, want 0", count)
	}
}

// TestBulkDeleteOptimized_StreamBacked_LargeBatch verifies that 2k+ stream-backed deletes
// complete in reasonable time via the batched stream-deleted path (BatchAddStreamDeletedIDs),
// so retention can delete at least 1k–5k objects per batch.
func TestBulkDeleteOptimized_StreamBacked_LargeBatch(t *testing.T) {

	projectRoot, storage2Ptr := setupBulkDeleteStreamTest(t)
	if err := storage.EnsurePathAliasCacheReady(projectRoot); err != nil {
		t.Fatalf("EnsurePathAliasCacheReady: %v", err)
	}
	createdAt, _ := time.Parse(time.RFC3339, "2030-03-01T12:00:00Z")

	const N = 500
	var ids []string
	for i := 1; i <= N; i++ {
		id := "AUD-BATCH-" + strconv.Itoa(i)
		ids = append(ids, id)
		obj := map[string]any{
			objects.FieldKeyID:        id,
			objects.FieldKeyKind:      "audit_event",
			objects.FieldKeyEventType: "object_creation",
			objects.FieldKeyCreatedAt: createdAt.Format(time.RFC3339),
		}
		seg, offset, err := storage.AppendToStream(projectRoot, "audit_event", id, obj, createdAt)
		if err != nil {
			t.Fatalf("AppendToStream %s: %v", id, err)
		}
		loc := storage.FormatStreamLocation(seg, offset)
		if err := storage.AppendStreamLocationToRegistry(projectRoot, "audit_event", id, loc); err != nil {
			t.Fatalf("AppendStreamLocationToRegistry %s: %v", id, err)
		}
	}

	var err error
	*storage2Ptr, err = storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	storage2 := *storage2Ptr

	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())
	secCtx := pkgctx.NewSystemSecurityContext()

	start := time.Now()
	res, err := storage2.BulkDeleteOptimized(ctx, secCtx, ids, false, 4)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("BulkDeleteOptimized: %v", err)
	}
	if res.SuccessCount != N {
		t.Errorf("BulkDeleteOptimized: SuccessCount = %d, want %d", res.SuccessCount, N)
	}
	if elapsed > 30*time.Second {
		t.Errorf("BulkDeleteOptimized %d deletes took %v (want < 30s for batched path)", N, elapsed)
	}
	filter := storage.ListFilter{Kind: "audit_event"}
	count, _ := storage2.Count(ctx, secCtx, filter)
	if count != 0 {
		t.Errorf("Count after bulk delete = %d, want 0", count)
	}
}
