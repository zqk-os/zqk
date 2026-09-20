package storage

import (
	"context"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// FindDependentsForTest exposes [FileObjectStorage.findDependents] for external tests.
func FindDependentsForTest(f *FileObjectStorage, ctx context.Context, id, kind string) ([]string, error) {
	return f.findDependents(ctx, id, kind)
}

// GetContentAddressableStorageForTest exposes [FileObjectStorage.getContentAddressableStorage] for debug-only test paths.
func GetContentAddressableStorageForTest(f *FileObjectStorage, kind string) (*filecas.ContentAddressableStorage, error) {
	return f.getContentAddressableStorage(kind)
}
