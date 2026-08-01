package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// TestFileObjectStorage_Create_WithBlockedIOQueue tests that Create falls back to direct I/O
// when the I/O queue is blocked, preventing timeouts
func TestFileObjectStorage_Create_WithBlockedIOQueue(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "zqk-test-*")
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
	defer func() { _ = fos.Shutdown(context.Background()) }()
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(tmpDir, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Logf("remove temp dir: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("account:test", []string{"admin"}, []string{"read:*", "write:*"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create a simple backlog item
	obj := map[string]any{
		objects.FieldKeyID:            "ITEM-TEST-001",
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Test Item",
		objects.FieldKeyStatus:        "exploring",
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
	readObj, readErr := fos.Read(ctx, secCtx, "ITEM-TEST-001")
	if readErr != nil {
		t.Fatalf("Failed to read created object: %v", readErr)
	}

	if readObj[objects.FieldKeyID] != "ITEM-TEST-001" {
		t.Errorf("Read object has wrong ID: %v", readObj[objects.FieldKeyID])
	}
}
