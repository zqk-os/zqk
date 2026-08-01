package storage

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

// SetupTestingFactoryCompleteTestEnvironmentForTest exposes [setupTestingFactoryCompleteTestEnvironment]
// for external test packages (e.g. storage_test).
func SetupTestingFactoryCompleteTestEnvironmentForTest(t *testing.T) (testRoot string, fos *FileObjectStorage, secCtx *pkgctx.SecurityContext) {
	return setupTestingFactoryCompleteTestEnvironment(t)
}

// SetupStorageInitBudgetTestEnvironmentForTest exposes [setupStorageInitBudgetTestEnvironmentForTest]
// for external test packages (e.g. storage_test init budget / bench).
func SetupStorageInitBudgetTestEnvironmentForTest(t *testing.T) string {
	t.Helper()
	return setupStorageInitBudgetTestEnvironmentForTest(t)
}
