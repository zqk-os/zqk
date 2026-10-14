package object

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// setupInProcessTestContext initializes an isolated temp project and storage provider for in-process testing.
func setupInProcessTestContext(t *testing.T) (string, storage.ObjectStorageProvider, *pkgctx.SecurityContext, func()) {
	t.Helper()
	tempDir, cleanup := setupTestProject(t)
	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		cleanup()
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)
	secCtx := pkgctx.NewSystemSecurityContext()
	return tempDir, provider, secCtx, cleanup
}
