package system

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStateRestoreStorageUnwrap(t *testing.T) {
	tmpDir := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	factory, err := storage.NewStorageFactory(t.Context(), tmpDir)
	if err != nil {
		t.Fatalf("failed to create StorageFactory: %v", err)
	}
	t.Cleanup(func() {
		if err := factory.Shutdown(context.Background()); err != nil {
			t.Logf("factory shutdown: %v", err)
		}
	})

	provider := factory.GetStorage()
	fileStorage := storage.UnwrapToFileObjectStorage(provider)
	if fileStorage == nil {
		t.Fatalf("UnwrapToFileObjectStorage returned nil for factory default storage")
	}

	if fileStorage.GetProjectRoot() != tmpDir {
		t.Errorf("expected project root %s, got %s", tmpDir, fileStorage.GetProjectRoot())
	}
}
