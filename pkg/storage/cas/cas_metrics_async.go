package cas

import (
	"context"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// CASMetricsAsyncCollector provides async metrics collection
// Metrics are batched and collected in the background to avoid blocking operations
// This is critical for high-volume CAS operations
type CASMetricsAsyncCollector struct {
	collector *CASMetricsCollector
	batchChan chan *casMetricsBatch
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	enabled   bool
	stopped   bool
}

type casMetricsBatch struct {
	windowStart time.Time
	windowEnd   time.Time
	ctx         context.Context
	secCtx      *pkgctx.SecurityContext
	callback    func(string, error)
}

var (
	globalCASAsyncCollector *CASMetricsAsyncCollector
	casAsyncCollectorOnce   sync.Once
)

// GetCASMetricsAsyncCollector returns the global async CAS metrics collector
func GetCASMetricsAsyncCollector(storage MetricsStorageFacade) *CASMetricsAsyncCollector {
	casAsyncCollectorOnce.Do(func() {
		globalCASAsyncCollector = NewCASMetricsAsyncCollector(storage)
	})
	return globalCASAsyncCollector
}

// ObjectStorageMetricsAsyncCollector is a type alias for [CASMetricsAsyncCollector].
type ObjectStorageMetricsAsyncCollector = CASMetricsAsyncCollector

// GetObjectStorageMetricsAsyncCollector returns the global async metrics collector (same as [GetCASMetricsAsyncCollector]).
func GetObjectStorageMetricsAsyncCollector(storage MetricsStorageFacade) *ObjectStorageMetricsAsyncCollector {
	return GetCASMetricsAsyncCollector(storage)
}

// NewObjectStorageMetricsAsyncCollector creates a new async metrics collector (same as [NewCASMetricsAsyncCollector]).
func NewObjectStorageMetricsAsyncCollector(storage MetricsStorageFacade) *ObjectStorageMetricsAsyncCollector {
	return NewCASMetricsAsyncCollector(storage)
}

// NewCASMetricsAsyncCollector creates a new async CAS metrics collector
func NewCASMetricsAsyncCollector(storage MetricsStorageFacade) *CASMetricsAsyncCollector {
	collector := &CASMetricsAsyncCollector{
		collector: NewObjectStorageMetricsCollector(storage),
		batchChan: make(chan *casMetricsBatch, 100), // Buffer up to 100 batches
		stopChan:  make(chan struct{}),
		enabled:   true,
	}

	// Start background worker
	goroutinelabels.NewGoroutine(ConstStreamCasMetricsAsyncWorker, ConstStreamProcessingAsyncCasMetricsCollection).
		WithWaitGroup(&collector.wg).
		StartSimple(collector.worker)

	return collector
}

// CollectMetricsAsync queues metrics collection to be processed asynchronously
// This method returns immediately without blocking - critical for high-volume CAS operations
func (c *CASMetricsAsyncCollector) CollectMetricsAsync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
	callback func(metricID string, err error),
) error {
	return concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameCasMetricsAsyncCollect, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if !c.enabled {
			return nil
		}
		select {
		case c.batchChan <- &casMetricsBatch{
			windowStart: windowStart,
			windowEnd:   windowEnd,
			ctx:         ctx,
			secCtx:      secCtx,
			callback:    callback,
		}:
			return nil
		default:
			return nil
		}
	})
}

// worker processes metrics batches in the background
func (c *CASMetricsAsyncCollector) worker() {
	for {
		select {
		case <-c.stopChan:
			// Process remaining batches before stopping
			for {
				select {
				case batch := <-c.batchChan:
					c.processBatch(batch)
				default:
					return
				}
			}
		case batch := <-c.batchChan:
			c.processBatch(batch)
		}
	}
}

// processBatch processes a single metrics batch
func (c *CASMetricsAsyncCollector) processBatch(batch *casMetricsBatch) {
	// Call the synchronous collector (this will be async relative to the caller)
	metricID, err := c.collector.CollectAndReset(batch.ctx, batch.secCtx, batch.windowStart, batch.windowEnd)

	// Invoke callback if provided
	if batch.callback != nil {
		batch.callback(metricID, err)
	}
}

// Stop stops the async collector and processes remaining batches
func (c *CASMetricsAsyncCollector) Stop() {
	var shouldStop bool
	if err := concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameCasMetricsAsyncCollectorStopCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		if c.stopped {
			return nil
		}
		shouldStop = true
		c.stopped = true
		c.enabled = false
		close(c.stopChan)
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamLockFailedInStopValN, err).Log()
	}

	if !shouldStop {
		return // Already stopped
	}

	// Wait for worker to finish (don't hold mutex during wait to avoid deadlock)
	c.wg.Wait()
}

// Enable enables async collection
func (c *CASMetricsAsyncCollector) Enable() {
	if err := concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameCasMetricsAsyncCollectorEnable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		c.enabled = true
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

			// Disable disables async collection (drops batches)
			Error(ConstStreamLockFailedInEnableValN, err).Log()
	}
}

func (c *CASMetricsAsyncCollector) Disable() {
	if err := concurrency.RunInLockWithLogger(&c.mu, locknames.LockNameCasMetricsAsyncCollectorDisable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)), func() error {
		c.enabled = false
		return nil
	}); err != nil {
		logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ConstStreamFailedToDisableAsyncCasMetricsCollectorValN, err).Log()
	}
}
