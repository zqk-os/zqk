package system

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/storage"
)

// drainHashRegistriesForTest stops and drains the given hash registries (background save workers).
func drainHashRegistriesForTest(t *testing.T, registries []*storage.HashRegistry) {
	t.Helper()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, hr := range registries {
		if hr == nil {
			continue
		}
		_ = hr.InitiateShutdown() //nolint:errcheck // test teardown
		_ = hr.Drain(shutdownCtx) //nolint:errcheck
	}
}

// drainGlobalStorageQueuesForTest drains process-wide shutdown coordinator, IO queue, and audit buffer.
// Call after per-test hash registries are drained (same ordering as pkg/coordination coordinator_integration_test).
func drainGlobalStorageQueuesForTest() {
	shutdownCoordinator := storage.GetGlobalShutdownCoordinator()
	if shutdownCoordinator != nil {
		_ = shutdownCoordinator.InitiateShutdown() //nolint:errcheck // test teardown
	}

	ioQueueManager := storage.GetGlobalIOQueueManager(pkgctx.NewSystemContext())
	if ioQueueManager != nil {
		_ = ioQueueManager.InitiateShutdown() //nolint:errcheck // test teardown
		ctx, cancelIO := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
		_ = ioQueueManager.Drain(ctx) //nolint:errcheck
		cancelIO()
	}

	buffer := storage.GetGlobalAuditEventBuffer()
	if buffer != nil {
		_ = buffer.Shutdown() //nolint:errcheck // test teardown
	}
}

// runHashRegistryAndGlobalTeardown drains hash-registry save workers then global storage queues
// so t.TempDir cleanup does not race background writers. Use from defer or t.Cleanup after all
// registries in the test are constructed.
//
// See also runAsyncValidationTestTeardown in async_check_deadlock_test.go (validator + pipeline).
func runHashRegistryAndGlobalTeardown(t *testing.T, registries []*storage.HashRegistry) {
	t.Helper()
	drainHashRegistriesForTest(t, registries)
	drainGlobalStorageQueuesForTest()
}
