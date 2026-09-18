package storage

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAppendDeleteToWAL(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	appendErr := storage.appendDeleteToWAL(objects.KindBacklogItem, "BLI-TEST-999")
	if appendErr != nil {
		t.Fatalf("unexpected error appending delete to WAL: %v", appendErr)
	}
}

func TestDeleteImpl_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	storage, err := NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSecurityContext("admin", []string{"delete:backlog_item"}, nil)

	delErr := storage.deleteImpl(ctx, secCtx, "BLI-NONEXISTENT", false)
	if delErr == nil {
		t.Fatal("expected error deleting nonexistent object, got nil")
	}
}
