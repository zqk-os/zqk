package storage

import (
	"context"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/storagetesting"
)

var testingFactoryOnce sync.Once

// TestingFactory implements [storagetesting.IsolationFactory] for use with
// [github.com/lanceman/zqk/pkg/testing.SetupCompleteTestEnvironment] or
// [SetupTestingFactoryCompleteTestEnvironmentForTest] from package storage_test (no pkg/testing import).
type TestingFactory struct{}

// NewTestingFactory creates a new testing factory for use in test setup.
// Prefer SetupTestingFactoryCompleteTestEnvironmentForTest (package storage_test) or
// setupTestingFactoryCompleteTestEnvironment (package storage tests).
// Example with pkg/testing (callers that still import it):
//
//	import (
//	    testconfig "github.com/lanceman/zqk/pkg/testing"
//	    storagepkg "github.com/lanceman/zqk/pkg/storage"
//	)
//
//	func TestMyFeature(t *testing.T) {
//	    factory := storagepkg.NewTestingFactory()
//	    env := testconfig.SetupCompleteTestEnvironment(t, nil, factory)
//	    defer env.Cleanup()
//
//	    storage := env.Storage.(storagepkg.ObjectStorageProvider)
//	    // Use storage...
//	}
func NewTestingFactory() *TestingFactory {
	return &TestingFactory{}
}

// CreateFileStorage creates a file-based storage provider with a per-test orphan cleanup queue.
// Implements [storagetesting.IsolationFactory].
// Each test gets its own queue and storage; no globals are shared. Use env.Storage.(*FileObjectStorage).GetOrphanCleanupQueue()
// when a test needs the queue. Cleanup (storage + queue shutdown) is registered via GetTestCleanup() in test setup.
func (f *TestingFactory) CreateFileStorage(testRoot string) (any, error) {
	testingFactoryOnce.Do(func() {
		SetListingIndexWriteQueueFactoryToPerProjectRoot()
	})
	fileStorage, err := NewFileObjectStorageForTest(testRoot)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(pkgctx.NewSystemContext())
	queue := &CASOrphanCleanupQueue{
		queue:     make(chan *orphanCleanupRequest, 1000),
		ctx:       ctx,
		cancel:    cancel,
		wgManager: NewWaitGroupManager(),
		metrics:   GetObjectStorageMetrics(),
		secCtx:    pkgctx.NewSystemSecurityContext(),
		logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fileStorage)
	fileStorage.SetOrphanCleanupQueue(queue)
	return fileStorage, nil
}

// GetAuditBuffer returns the global audit event buffer.
// Implements [storagetesting.IsolationFactory].
func (f *TestingFactory) GetAuditBuffer() any {
	return GetGlobalAuditEventBuffer()
}

// ConfigureTestGlobals implements [storagetesting.GlobalHookConfigurator].
// When opts.NoopStorageMetrics is false, opts in to pkg/metricsrecording
// for this test (storage counters + pipeline metric creation). Default true leaves the
// global test-binary default (recording off unless ZQK_TEST_METRICS_RECORDING is set).
func (f *TestingFactory) ConfigureTestGlobals(t *testing.T, opts *storagetesting.GlobalHookOptions) {
	if opts == nil {
		opts = &storagetesting.GlobalHookOptions{NoopStorageMetrics: true}
	}
	if opts.NoopStorageMetrics {
		return
	}
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)
}
