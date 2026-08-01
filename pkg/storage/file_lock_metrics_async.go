package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

// FileLockMetricsAsyncCollector provides async metrics collection
// Metrics are batched and collected in the background to avoid blocking operations
type FileLockMetricsAsyncCollector struct {
	collector *FileLockMetricsCollector
	batchChan chan *metricsBatch
	stopChan  chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	enabled   atomic.Bool
	stopped   atomic.Bool
	stopOnce  sync.Once // Ensure Stop() only waits once

	batchesEnqueuedTotal atomic.Int64
	batchesDroppedTotal  atomic.Int64
}

// GetAsyncCollectorStats returns lifetime counters for batches enqueued and dropped.
func (c *FileLockMetricsAsyncCollector) GetAsyncCollectorStats() (enqueued, dropped int64) {
	if c == nil {
		return 0, 0
	}
	return c.batchesEnqueuedTotal.Load(), c.batchesDroppedTotal.Load()
}

type metricsBatch struct {
	windowStart time.Time
	windowEnd   time.Time
	ctx         context.Context
	secCtx      *pkgctx.SecurityContext
	callback    func(string, error)
}

var globalAsyncCollector *FileLockMetricsAsyncCollector
var asyncCollectorOnce sync.Once

// GetFileLockMetricsAsyncCollector returns the global async metrics collector
func GetFileLockMetricsAsyncCollector(storage ObjectStorageProvider) *FileLockMetricsAsyncCollector {
	asyncCollectorOnce.Do(func() {
		globalAsyncCollector = NewFileLockMetricsAsyncCollector(storage)
	})
	return globalAsyncCollector
}

// NewFileLockMetricsAsyncCollector creates a new async metrics collector
func NewFileLockMetricsAsyncCollector(storage ObjectStorageProvider) *FileLockMetricsAsyncCollector {
	collector := &FileLockMetricsAsyncCollector{
		collector: NewFileLockMetricsCollector(storage),
		batchChan: make(chan *metricsBatch, 100), // Buffer up to 100 batches
		stopChan:  make(chan struct{}),
	}
	collector.enabled.Store(true)

	// Start background worker
	goroutinelabels.NewGoroutine(ConstMiscFileLockMetricsAsyncWorker, ConstMiscProcessingAsyncFileLockMetricsCollection).
		WithWaitGroup(&collector.wg).
		StartSimple(collector.worker)

	return collector
}

// CollectMetricsAsync queues metrics collection to be processed asynchronously
// This method returns immediately without blocking
func (c *FileLockMetricsAsyncCollector) CollectMetricsAsync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
	callback func(metricID string, err error),
) error {
	return concurrency.RunInLockWithLogger(
		&c.mu, locknames.LockNameFileLockMetricsAsyncCollectorEnqueue, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if !c.enabled.Load() {
				return nil // Silently skip if disabled
			}

			select {
			case c.batchChan <- &metricsBatch{
				windowStart: windowStart,
				windowEnd:   windowEnd,
				ctx:         ctx,
				secCtx:      secCtx,
				callback:    callback,
			}:
				c.batchesEnqueuedTotal.Add(1)
				return nil
			default:
				// Channel is full - drop batch to avoid blocking
				// In production, you might want to log this or use a larger buffer
				c.batchesDroppedTotal.Add(1)
				return nil
			}
		},
	)
}

// worker processes metrics batches in the background
// Note: WaitGroup Done() is called by the goroutine builder, not here
func (c *FileLockMetricsAsyncCollector) worker() {
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
func (c *FileLockMetricsAsyncCollector) processBatch(batch *metricsBatch) {
	// Call the synchronous collector (this will be async relative to the caller)
	metricID, err := c.collector.CollectAndReset(batch.ctx, batch.secCtx, batch.windowStart, batch.windowEnd)

	// Invoke callback if provided
	if batch.callback != nil {
		batch.callback(metricID, err)
	}
}

// Stop stops the async collector and processes remaining batches
// Safe to call multiple times (idempotent)
func (c *FileLockMetricsAsyncCollector) Stop() {
	var shouldStop bool
	logger := logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := concurrency.RunInLockWithLogger(
		&c.mu, locknames.LockNameFileLockMetricsAsyncCollectorStopCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if c.stopped.Load() {
				return nil
			}
			shouldStop = true
			c.stopped.Store(true)
			c.enabled.Store(false)
			close(c.stopChan)
			return nil
		},
	)
	if err != nil {
		if logger != nil {
			logger.Warn(LogEventStorageFileLockAsyncStopLockTimeout,
				concurrency.LockField{Key: "error", Value: err.Error()})
		}
		return
	}

	if !shouldStop {
		return // Already stopped
	}

	// Wait for worker to finish exactly once (don't hold mutex during wait to avoid deadlock)
	c.stopOnce.Do(func() {
		c.wg.Wait()
	})
}

// Enable enables async collection
func (c *FileLockMetricsAsyncCollector) Enable() {
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameFileLockMetricsAsyncCollectorEnable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.enabled.Store(true)
			return nil
		},
	)
}

// Disable disables async collection (drops batches)
func (c *FileLockMetricsAsyncCollector) Disable() {
	_ = concurrency.RunInLockOrLog(
		&c.mu, locknames.LockNameFileLockMetricsAsyncCollectorDisable, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.enabled.Store(false)
			return nil
		},
	)
}
