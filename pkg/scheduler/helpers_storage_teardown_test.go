package scheduler

import (
	"testing"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// registerFileStorageTestTeardown registers standard storage test teardown and
// force-removes the projectRoot so t.TempDir cleanup never fails with unlinkat ENOTEMPTY.
func registerFileStorageTestTeardown(t *testing.T, projectRoot string, st *storagepkg.FileObjectStorage) {
	t.Helper()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(projectRoot); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	testkit.RegisterStorageTestCleanup(t, projectRoot, st)
}

// registerProjectDirTestTeardown registers standard storage test teardown (for nil storage)
// and force-removes the projectRoot so t.TempDir cleanup never fails with unlinkat ENOTEMPTY.
func registerProjectDirTestTeardown(t *testing.T, projectRoot string) {
	t.Helper()
	t.Cleanup(func() {
		if err := fileutil.RemoveAll(projectRoot); err != nil {
			t.Logf("cleanup: %v", err)
		}
	})
	testkit.RegisterStorageTestCleanup(t, projectRoot, nil)
}
