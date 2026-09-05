//go:build !production
// +build !production

package storage

import (
	"context"
	"testing"
)

// UsesContentAddressableStorageForTest exposes usesContentAddressableStorage for tests.
func (f *FileObjectStorage) UsesContentAddressableStorageForTest(kind string) bool {
	return f.usesContentAddressableStorage(kind)
}

// ValidateObjectForTest exposes validateObject for tests.
func (f *FileObjectStorage) ValidateObjectForTest(ctx context.Context, obj map[string]any, kind, currentState string) error {
	return f.validateObject(ctx, obj, kind, currentState)
}

// GetObjectFilePathForTest exposes getObjectFilePath for tests.
func (f *FileObjectStorage) GetObjectFilePathForTest(id, kind string) (string, error) {
	return f.getObjectFilePath(id, kind)
}

// DisableStreamStorageForTest disables stream storage for CAS tests.
func DisableStreamStorageForTest(t *testing.T) {
	t.Helper()
	t.Setenv("ZQK_STREAM_STORAGE_ENABLED", "0")
}

func RemoveOrphanCASFilesForObjectIDsForTest(want map[string]struct{}, kindDir string) {
	removeOrphanCASFilesForObjectIDs(want, kindDir)
}
