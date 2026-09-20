package scheduler

import (
	"testing"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// prepareHandlersIsolatedTempProject runs the standard isolated temp-project pipeline for handler
// tests ([testkit.PrepareIsolatedTempProject]). Callers must not use [testing.T.Parallel] on the
// same *testing.T.
func prepareHandlersIsolatedTempProject(t *testing.T) (testRoot string, fileStorage *storagepkg.FileObjectStorage) {
	t.Helper()
	p := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{Kind: "pkg.scheduler.handlers"})
	return p.Root, p.FileStorage
}
