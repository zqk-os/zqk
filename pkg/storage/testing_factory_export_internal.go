//go:build !production

package storage

import (
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

// NewStorageFactoryForTesting returns a StorageFactory backed by the provided ObjectStorageProvider
func NewStorageFactoryForTesting(provider ObjectStorageProvider) *StorageFactory {
	return &StorageFactory{
		defaultStorage: provider,
		providers:      make(map[string]ObjectStorageProvider),
	}
}

