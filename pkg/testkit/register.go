package testkit

import (
	"testing"

	"github.com/lanceman/zqk/pkg/storage"
)

const emptyValue = ""

// RegisterTempProjectTeardown registers t.Cleanup using [TempProjectTeardown] (strip, scrub, CAS/WAL, 45s timeouts).
// For audit-buffer teardown or other overrides, build [TeardownOptions] with [TempProjectTeardown] and call [RegisterStandardTeardown].
func RegisterTempProjectTeardown(t testing.TB, projectRoot string, fileStorage *storage.FileObjectStorage) {
	t.Helper()
	if projectRoot == emptyValue || fileStorage == nil {
		return
	}
	RegisterStandardTeardown(t, TempProjectTeardown(projectRoot, fileStorage))
}

// RegisterStandardTeardown registers t.Cleanup that runs [RunStandardTeardown] with defaults applied.
func RegisterStandardTeardown(t testing.TB, opts TeardownOptions) {
	t.Helper()
	t.Cleanup(func() {
		if err := RunStandardTeardown(opts); err != nil {
			t.Logf("testkit: storage teardown pipeline: %v", err)
		}
	})
}

// RegisterStorageTestCleanup registers teardown for storage returned by [storage.NewTestingFactory]
// (GetTestCleanup when present) or falls back to [RegisterStandardTeardown] with FileStorage.
func RegisterStorageTestCleanup(t testing.TB, projectRoot string, storageAny any) {
	t.Helper()
	if storageAny == nil || projectRoot == emptyValue {
		return
	}
	type cleanupGetter interface {
		GetTestCleanup() func()
	}
	if g, ok := storageAny.(cleanupGetter); ok {
		if fn := g.GetTestCleanup(); fn != nil {
			t.Cleanup(fn)
			return
		}
	}
	fs, ok := storageAny.(*storage.FileObjectStorage)
	if ok && fs != nil {
		RegisterTempProjectTeardown(t, projectRoot, fs)
		return
	}
	RegisterStandardTeardown(t, TeardownOptions{ProjectRoot: projectRoot})
}
