package scheduler

import (
	"testing"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// registerFileStorageTestTeardown registers [storagepkg.RunProjectTestTeardown] with
// [storagepkg.TempProjectTeardown] after [storagepkg.NewFileObjectStorageForTest].
// Register before other t.Cleanup callbacks that stop routers or handlers so those
// run first (cleanups execute in reverse registration order; test defers run before cleanups).
func registerFileStorageTestTeardown(t *testing.T, projectRoot string, st *storagepkg.FileObjectStorage) {
	t.Helper()
	t.Cleanup(func() {
		opts := storagepkg.TempProjectTeardown(projectRoot, st)
		if err := storagepkg.RunProjectTestTeardown(opts); err != nil {
			t.Logf("project test teardown: %v", err)
		}
	})
}
