package storage

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	// RegisterStorageCleanup registers t.Cleanup to call Shutdown on the given storage if it is
	// *FileObjectStorage. Use with NewFileObjectStorageForTest so hash registry workers are drained
	// and tests do not leak goroutines. No-op if storage is not *FileObjectStorage.
	//
	// For temp project roots that need the full teardown pipeline (listing queues, WAL, audit buffer,
	// strip/scrub), prefer t.Cleanup with [TempProjectTeardown] and [RunProjectTestTeardown] in pkg/storage tests.
)

func RegisterStorageCleanup(t *testing.T, storage any) {
	t.Helper()
	if f, ok := storage.(*FileObjectStorage); ok {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second) // Background: request-or-shutdown derived
			defer cancel()
			var _err_84096214 = f.Shutdown(ctx)
			if _err_84096214 !=

				// drainHashRegistryForTest drains a HashRegistry worker for test cleanup
				// This ensures workers don't continue running after tests complete, preventing
				// resource contention in parallel test execution.
				nil {
				logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84096214).Log()
			}
		})
	}
}

func drainHashRegistryForTest(registry *HashRegistry) {
	if registry == nil {
		return
	}

	// Create a short timeout context for test cleanup
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 2*time.Second)
	defer cancel()
	var _err_84096603 = registry.Drain(ctx)
	if _err_84096603 != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84096603).Log()
	}
}
