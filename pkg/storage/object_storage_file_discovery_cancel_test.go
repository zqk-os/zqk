package storage

import (
	"errors"

	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestEnsureCASIndexPopulatedFromScan_WithCanceledContext_NoPanic verifies that
// EnsureCASIndexPopulatedFromScan accepts a context and does not panic when the
// context is already cancelled (SCH-002 fix: scan must respect context so job timeout aborts long scans).
func TestEnsureCASIndexPopulatedFromScan_WithCanceledContext_NoPanic(t *testing.T) {
	dir := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(dir)
	dirName := objects.GetDirectoryFromKind("audit_event")
	if dirName == emptyValue {
		dirName = "audit"
	}
	auditDir := filepath.Join(processDir, dirName)
	if err := fileutil.MkdirAll(auditDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	// Use ForTest so WAL/write-behind are not started; full NewFileObjectStorage leaves
	// .zqk/wal populated and t.TempDir() cleanup fails with "directory not empty" under parallel runs.
	f, err := NewFileObjectStorageForTest(dir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = f.Shutdown(context.Background()) }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err = f.EnsureCASIndexPopulatedFromScan(ctx, "audit_event")
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("expected nil or context.Canceled, got %v", err)
	}
}
