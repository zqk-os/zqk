package objects

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// CacheUpdateType represents the type of cache update
type CacheUpdateType string

const (
	CacheUpdateSpec        CacheUpdateType = "spec"         // Update spec cache (ontology -> cachedSpec)
	CacheUpdatePath        CacheUpdateType = "path"         // Update path cache (filePath -> cachedSpec)
	CacheUpdateOntology    CacheUpdateType = "ontology"     // Update ontology cache (filePath -> ontology)
	CacheUpdateFileContent CacheUpdateType = "file_content" // Update file content cache (filePath -> fileContentEntry)
)

// CacheUpdate represents a single cache update operation
type CacheUpdate struct {
	Type      CacheUpdateType
	Key       string
	Value     any
	Shard     *specShard
	Timestamp time.Time
}

// SpecCacheUpdateBuffer queues cache updates and applies them in batches to reduce sync.Map contention
// Write-behind pattern: updates are queued and applied by a background worker in bursts
type SpecCacheUpdateBuffer struct {
	updates   chan *CacheUpdate
	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	doneCh    chan struct{}
	batchSize int
	interval  time.Duration
	logger    logging.Logger
	// workerActive tracks if worker is actively processing (not idle waiting)
	workerActive int32 // atomic: 1 = active, 0 = idle
}

// NewSpecCacheUpdateBuffer creates a new cache update buffer with write-behind batching
func NewSpecCacheUpdateBuffer(bufferSize, batchSize int, interval time.Duration) *SpecCacheUpdateBuffer {
	return &SpecCacheUpdateBuffer{
		updates:   make(chan *CacheUpdate, bufferSize),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
		batchSize: batchSize,
		interval:  interval,
		logger:    logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Start starts the background worker that batches and applies cache updates
func (b *SpecCacheUpdateBuffer) Start(ctx context.Context) {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.mu.Unlock()

	bud := goroutinelabels.DefaultBudget()
	goroutinelabels.NewGoroutine("spec_cache_update_worker", "batching and applying spec cache updates").
		WithContext(ctx).
		WithBudget(bud).
		StartSimple(func() {
			defer close(b.doneCh)
			b.worker(ctx)
		})
}

// Stop stops the worker and drains remaining updates
func (b *SpecCacheUpdateBuffer) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	b.mu.Unlock()

	close(b.stopCh)
	<-b.doneCh
}

// Enqueue queues a cache update (non-blocking: falls back to direct Store() if buffer is full AND worker is active)
// This prevents deadlock: if worker is blocked in applyBatch, fallback ensures progress
func (b *SpecCacheUpdateBuffer) Enqueue(update *CacheUpdate) {
	select {
	case b.updates <- update:
		// Successfully queued - will be batched
	default:
		// Buffer full - check if worker is actively processing
		// If worker is idle (waiting), it will drain soon - block briefly
		// If worker is active but buffer still full, worker might be blocked - fallback
		if atomic.LoadInt32(&b.workerActive) == 1 {
			// Worker is active but buffer full - likely blocked in applyBatch
			// Fallback to direct Store() to prevent deadlock
			b.applyUpdateToShard(update.Shard, update)
		} else {
			// Worker is idle - it will drain soon, so block briefly
			// Use a short timeout to prevent indefinite blocking
			select {
			case b.updates <- update:
				// Successfully queued after brief wait
			case <-time.After(10 * time.Millisecond):
				// Worker didn't drain in time - fallback to direct Store()
				b.applyUpdateToShard(update.Shard, update)
			}
		}
	}
}

// worker processes updates in batches
func (b *SpecCacheUpdateBuffer) worker(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	batch := make([]*CacheUpdate, 0, b.batchSize)
	lastFlush := time.Now()

	for {
		// Mark worker as active while draining/processing
		atomic.StoreInt32(&b.workerActive, 1)

		// Drain as many updates as possible without blocking
		drained := false
		for len(batch) < b.batchSize {
			select {
			case update := <-b.updates:
				batch = append(batch, update)
				drained = true
			default:
				// No more updates available immediately
				goto checkFlush
			}
		}

	checkFlush:
		// Apply batch if full or time-based flush needed
		now := time.Now()
		shouldFlush := len(batch) >= b.batchSize || (len(batch) > 0 && now.Sub(lastFlush) >= b.interval)

		if shouldFlush && len(batch) > 0 {
			b.applyBatch(batch)
			batch = batch[:0] // Reset slice but keep capacity
			lastFlush = now
			drained = false // Reset - we just flushed
		}

		// If we didn't drain anything, wait for updates or ticker
		if !drained {
			// Mark worker as idle while waiting
			atomic.StoreInt32(&b.workerActive, 0)
			select {
			case <-ctx.Done():
				// Apply remaining batch before exit
				atomic.StoreInt32(&b.workerActive, 1)
				b.applyBatch(batch)
				return
			case <-b.stopCh:
				// Apply remaining batch before exit
				atomic.StoreInt32(&b.workerActive, 1)
				b.applyBatch(batch)
				return
			case update := <-b.updates:
				// Got an update - mark active again
				atomic.StoreInt32(&b.workerActive, 1)
				batch = append(batch, update)
			case <-ticker.C:
				// Time-based flush: apply batch even if not full
				atomic.StoreInt32(&b.workerActive, 1)
				if len(batch) > 0 {
					b.applyBatch(batch)
					batch = batch[:0]
					lastFlush = time.Now()
				}
			}
		}
	}
}

// applyBatch applies a batch of updates to reduce sync.Map contention
func (b *SpecCacheUpdateBuffer) applyBatch(batch []*CacheUpdate) {
	if len(batch) == 0 {
		return
	}

	// Group updates by shard to reduce contention
	shardUpdates := make(map[*specShard][]*CacheUpdate)
	for _, update := range batch {
		shardUpdates[update.Shard] = append(shardUpdates[update.Shard], update)
	}

	// Apply updates per shard (reduces contention on sync.Map)
	for shard, updates := range shardUpdates {
		for _, update := range updates {
			b.applyUpdateToShard(shard, update)
		}
	}
}

// applyUpdateToShard applies an update to a specific shard
func (b *SpecCacheUpdateBuffer) applyUpdateToShard(shard *specShard, update *CacheUpdate) {
	switch update.Type {
	case CacheUpdateSpec:
		shard.cache.Store(update.Key, update.Value)
	case CacheUpdatePath:
		shard.pathCache.Store(update.Key, update.Value)
	case CacheUpdateOntology:
		shard.ontologyCache.Store(update.Key, update.Value)
	case CacheUpdateFileContent:
		shard.fileContentCache.Store(update.Key, update.Value)
	}
}
