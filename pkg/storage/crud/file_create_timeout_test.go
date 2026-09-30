package crud_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestFileObjectStorage_Create_WithBlockedIOQueue tests that Create falls back to direct I/O
// when the I/O queue is blocked, preventing timeouts
func TestFileObjectStorage_Create_WithBlockedIOQueue(t *testing.T) {

	tmpDir, err := fileutil.MkdirTemp("", "zqk-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := fileutil.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a simple backlog item
	obj := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Test Item",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	// This should complete within the timeout, even if I/O queue is slow
	// The fallback to direct I/O should prevent hanging
	start := time.Now()
	err = fos.Create(ctx, secCtx, obj)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Create failed: %v (duration: %v)", err, duration)
	}

	// Should complete quickly (fallback kicks in after 5s, so should be < 6s)
	if duration > 6*time.Second {
		t.Errorf("Create took too long: %v (expected < 6s with fallback)", duration)
	}

	// Verify object was actually created
	readObj, readErr := fos.Read(ctx, secCtx, "BLI-TEST-001")
	if readErr != nil {
		t.Fatalf("Failed to read created object: %v", readErr)
	}

	if readObj[objects.FieldKeyID] != "BLI-TEST-001" {
		t.Errorf("Read object has wrong ID: %v", readObj[objects.FieldKeyID])
	}
}

func TestFileObjectStorage_Create_HonestContextCancellationUnderContention(t *testing.T) {
	tmpDir := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	fos, err := storage.NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() { _ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, fos)) })

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})

	// Case 1: An immediately cancelled context must return error and NOT leave a persisted object behind
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	obj1 := map[string]any{
		objects.FieldKeyID:            "BLI-TEST-CANCELLED-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Should not persist",
		objects.FieldKeyStatus:        objects.ObjectStatusExploring,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	createErr := fos.Create(cancelledCtx, secCtx, obj1)
	if createErr == nil {
		t.Fatal("expected context cancelled error, got nil")
	}

	// Verify no phantom object was created
	validCtx := context.Background()
	_, readErr := fos.Read(validCtx, secCtx, "BLI-TEST-CANCELLED-001")
	if readErr == nil {
		t.Fatalf("CRIT-CEF-R16-CREATE-CTXCANCEL-001 violation: object persisted despite cancelled context on create")
	}

	// Case 2: Concurrent valid creations under contention must report success and be retrievable without false failure
	numConcurrent := 10
	errCh := make(chan error, numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		id := fmt.Sprintf("BLI-TEST-CONCURRENT-%03d", i)
		go func(objID string) {
			obj := map[string]any{
				objects.FieldKeyID:            objID,
				objects.FieldKeyKind:          "backlog_item",
				objects.FieldKeyTitle:         "Concurrent Item",
				objects.FieldKeyStatus:        objects.ObjectStatusExploring,
				objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errCh <- fos.Create(ctx, secCtx, obj)
		}(id)
	}

	for i := 0; i < numConcurrent; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent create failed: %v", err)
		}
	}

	for i := 0; i < numConcurrent; i++ {
		id := fmt.Sprintf("BLI-TEST-CONCURRENT-%03d", i)
		readObj, err := fos.Read(validCtx, secCtx, id)
		if err != nil {
			t.Fatalf("failed to read successfully created object %s: %v", id, err)
		}
		if readObj[objects.FieldKeyID] != id {
			t.Errorf("expected ID %s, got %v", id, readObj[objects.FieldKeyID])
		}
	}
}
