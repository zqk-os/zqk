// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package batchaf

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Common error definitions for Batch AF subsystem.
var (
	ErrAggregatorClosed = errors.New("batchaf: aggregator closed")
	ErrBufferFull       = errors.New("batchaf: buffer queue full")
	ErrNilCallback      = errors.New("batchaf: flush callback cannot be nil")
	ErrInvalidConfig    = errors.New("batchaf: invalid configuration parameters")
	ErrBatchTimeout     = errors.New("batchaf: batch execution timed out")
)

// BatchStatus indicates the lifecycle phase of a batch.
type BatchStatus string

const (
	BatchStatusActive    BatchStatus = "active"
	BatchStatusInFlight  BatchStatus = "in_flight"
	BatchStatusSucceeded BatchStatus = "succeeded"
	BatchStatusRetrying  BatchStatus = "retrying"
	BatchStatusFailed    BatchStatus = "failed"
	BatchStatusReclaimed BatchStatus = "reclaimed"
)

// Batch represents an aggregated collection of items processed together.
type Batch[T any] struct {
	ID        string
	Items     []T
	Status    BatchStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	Attempts  int
	LastError error
}

// Options configure the BatchAggregator.
type Options[T any] struct {
	MaxBatchSize       int
	FlushInterval      time.Duration
	WorkerConcurrency  int
	BufferCapacity     int
	BatchTimeout       time.Duration
	MaxRetries         int
	BackoffBase        time.Duration
	FlushCallback      func(ctx context.Context, batch []T) error
	DeadLetterCallback func(batch *Batch[T], err error)
}

// DefaultOptions returns baseline configuration settings.
func DefaultOptions[T any]() Options[T] {
	return Options[T]{
		MaxBatchSize:      100,
		FlushInterval:     100 * time.Millisecond,
		WorkerConcurrency: 2,
		BufferCapacity:    1000,
		BatchTimeout:      5 * time.Second,
		MaxRetries:        3,
		BackoffBase:       10 * time.Millisecond,
	}
}

// Validate checks whether configuration values are sound.
func (o *Options[T]) Validate() error {
	if o.MaxBatchSize <= 0 {
		return fmt.Errorf("%w: MaxBatchSize must be > 0", ErrInvalidConfig)
	}
	if o.FlushInterval <= 0 {
		return fmt.Errorf("%w: FlushInterval must be > 0", ErrInvalidConfig)
	}
	if o.WorkerConcurrency <= 0 {
		return fmt.Errorf("%w: WorkerConcurrency must be > 0", ErrInvalidConfig)
	}
	if o.BufferCapacity <= 0 {
		return fmt.Errorf("%w: BufferCapacity must be > 0", ErrInvalidConfig)
	}
	if o.BatchTimeout <= 0 {
		return fmt.Errorf("%w: BatchTimeout must be > 0", ErrInvalidConfig)
	}
	if o.MaxRetries < 0 {
		return fmt.Errorf("%w: MaxRetries cannot be negative", ErrInvalidConfig)
	}
	if o.BackoffBase <= 0 {
		return fmt.Errorf("%w: BackoffBase must be > 0", ErrInvalidConfig)
	}
	if o.FlushCallback == nil {
		return ErrNilCallback
	}
	return nil
}

// BatchStats captures atomic execution metrics.
type BatchStats struct {
	ItemsSubmitted   uint64
	BatchesFlushed   uint64
	BatchesSucceeded uint64
	BatchesFailed    uint64
	BatchesRetried   uint64
	BatchesReclaimed uint64
	DeadLettered     uint64
}

// ErrorCollector safely aggregates asynchronous execution errors.
type ErrorCollector struct {
	mu     sync.RWMutex
	errors []error
}

// NewErrorCollector constructs an empty error collector.
func NewErrorCollector() *ErrorCollector {
	return &ErrorCollector{
		errors: make([]error, 0),
	}
}

// Collect records an error if non-nil.
func (ec *ErrorCollector) Collect(err error) {
	if err == nil {
		return
	}
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = append(ec.errors, err)
}

// Errors returns a copy of all accumulated errors.
func (ec *ErrorCollector) Errors() []error {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	out := make([]error, len(ec.errors))
	copy(out, ec.errors)
	return out
}

// Count returns the number of captured errors.
func (ec *ErrorCollector) Count() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.errors)
}

// Reset clears accumulated errors.
func (ec *ErrorCollector) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = make([]error, 0)
}

// TimerSentinel provides tick-based interval coordination for threshold flushing.
type TimerSentinel struct {
	interval time.Duration
	ticker   *time.Ticker
	tickCh   chan time.Time
	stopCh   chan struct{}
	stopped  atomic.Bool
	wg       sync.WaitGroup
}

// NewTimerSentinel creates a sentinel emitting regular ticks until stopped.
func NewTimerSentinel(interval time.Duration) *TimerSentinel {
	ts := &TimerSentinel{
		interval: interval,
		ticker:   time.NewTicker(interval),
		tickCh:   make(chan time.Time, 1),
		stopCh:   make(chan struct{}),
	}
	ts.wg.Add(1)
	goroutinelabels.NewGoroutine("batchaf_sentinel", "flush interval timer tick dispatch").StartSimple(func() {
		defer ts.wg.Done()
		for {
			select {
			case t, ok := <-ts.ticker.C:
				if !ok {
					return
				}
				select {
				case ts.tickCh <- t:
				default:
				}
			case <-ts.stopCh:
				return
			}
		}
	})
	return ts
}

// Ticks returns the read channel receiving periodic time notifications.
func (ts *TimerSentinel) Ticks() <-chan time.Time {
	return ts.tickCh
}

// Interval returns the configured tick interval.
func (ts *TimerSentinel) Interval() time.Duration {
	return ts.interval
}

// Stop terminates the internal ticker and flusher loop.
func (ts *TimerSentinel) Stop() {
	if ts.stopped.CompareAndSwap(false, true) {
		ts.ticker.Stop()
		close(ts.stopCh)
		ts.wg.Wait()
	}
}

// ReclamationQueue tracks active in-flight batches and isolates orphaned batches.
type ReclamationQueue[T any] struct {
	mu       sync.RWMutex
	inFlight map[string]*Batch[T]
	orphaned []*Batch[T]
}

// NewReclamationQueue creates a fresh queue for batch tracking.
func NewReclamationQueue[T any]() *ReclamationQueue[T] {
	return &ReclamationQueue[T]{
		inFlight: make(map[string]*Batch[T]),
		orphaned: make([]*Batch[T], 0),
	}
}

// RegisterInFlight marks a batch as dispatched to workers.
func (rq *ReclamationQueue[T]) RegisterInFlight(b *Batch[T]) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	b.Status = BatchStatusInFlight
	if b.UpdatedAt.IsZero() {
		b.UpdatedAt = time.Now()
	}
	rq.inFlight[b.ID] = b
}

// MarkSuccess marks a batch as completed and removes it from in-flight tracking.
func (rq *ReclamationQueue[T]) MarkSuccess(batchID string) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	if b, ok := rq.inFlight[batchID]; ok {
		b.Status = BatchStatusSucceeded
		b.UpdatedAt = time.Now()
		delete(rq.inFlight, batchID)
	}
}

// MarkFailed records batch failure and removes it from in-flight tracking.
func (rq *ReclamationQueue[T]) MarkFailed(batchID string, err error) {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	if b, ok := rq.inFlight[batchID]; ok {
		b.Status = BatchStatusFailed
		b.LastError = err
		b.UpdatedAt = time.Now()
		delete(rq.inFlight, batchID)
	}
}

// SweepOrphaned scans in-flight batches, identifying any exceeding the timeout threshold.
func (rq *ReclamationQueue[T]) SweepOrphaned(timeout time.Duration) []*Batch[T] {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	now := time.Now()
	var newlyOrphaned []*Batch[T]
	for id, b := range rq.inFlight {
		if now.Sub(b.UpdatedAt) >= timeout {
			b.Status = BatchStatusReclaimed
			b.UpdatedAt = now
			newlyOrphaned = append(newlyOrphaned, b)
			rq.orphaned = append(rq.orphaned, b)
			delete(rq.inFlight, id)
		}
	}
	return newlyOrphaned
}

// InFlightCount returns the count of batches currently undergoing processing.
func (rq *ReclamationQueue[T]) InFlightCount() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	return len(rq.inFlight)
}

// OrphanedCount returns the cumulative number of orphaned batches captured.
func (rq *ReclamationQueue[T]) OrphanedCount() int {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	return len(rq.orphaned)
}

// AllOrphaned returns a shallow copy of all orphaned batches.
func (rq *ReclamationQueue[T]) AllOrphaned() []*Batch[T] {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	out := make([]*Batch[T], len(rq.orphaned))
	copy(out, rq.orphaned)
	return out
}

// Get returns the in-flight batch matching the given identifier if present.
func (rq *ReclamationQueue[T]) Get(batchID string) (*Batch[T], bool) {
	rq.mu.RLock()
	defer rq.mu.RUnlock()
	b, ok := rq.inFlight[batchID]
	return b, ok
}

// WorkerPool coordinates parallel worker routines processing dispatched batches.
type WorkerPool[T any] struct {
	concurrency int
	jobCh       chan *Batch[T]
	doneCh      chan struct{}
	wg          sync.WaitGroup
	stopped     atomic.Bool
}

// NewWorkerPool instantiates a worker pool and launches worker goroutines.
func NewWorkerPool[T any](concurrency int, handler func(*Batch[T])) *WorkerPool[T] {
	wp := &WorkerPool[T]{
		concurrency: concurrency,
		jobCh:       make(chan *Batch[T], concurrency*2),
		doneCh:      make(chan struct{}),
	}
	for i := 0; i < concurrency; i++ {
		workerID := i
		wp.wg.Add(1)
		goroutinelabels.NewGoroutine("batchaf_worker", fmt.Sprintf("batch worker %d", workerID)).StartSimple(func() {
			defer wp.wg.Done()
			for job := range wp.jobCh {
				handler(job)
			}
		})
	}
	return wp
}

// Dispatch submits a batch job to the worker queue.
func (wp *WorkerPool[T]) Dispatch(job *Batch[T]) bool {
	if wp.stopped.Load() {
		return false
	}
	wp.jobCh <- job
	return true
}

// Stop shuts down the worker pool and awaits termination of all active worker routines.
func (wp *WorkerPool[T]) Stop() {
	if wp.stopped.CompareAndSwap(false, true) {
		close(wp.jobCh)
		wp.wg.Wait()
	}
}

// BatchAggregator manages continuous stream aggregation, interval flushes, and worker fanout.
type BatchAggregator[T any] struct {
	opts        Options[T]
	itemCh      chan T
	flushCh     chan chan error
	closeCh     chan struct{}
	drainDoneCh chan struct{}
	stopped     atomic.Bool
	batchSeq    atomic.Uint64

	stats       BatchStats
	sentinel    *TimerSentinel
	workers     *WorkerPool[T]
	reclamation *ReclamationQueue[T]
	errors      *ErrorCollector

	mu          sync.Mutex
	activeItems []T
}

// NewBatchAggregator validates options and initializes an operational BatchAggregator.
func NewBatchAggregator[T any](opts Options[T]) (*BatchAggregator[T], error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	ba := &BatchAggregator[T]{
		opts:        opts,
		itemCh:      make(chan T, opts.BufferCapacity),
		flushCh:     make(chan chan error, 16),
		closeCh:     make(chan struct{}),
		drainDoneCh: make(chan struct{}),
		sentinel:    NewTimerSentinel(opts.FlushInterval),
		reclamation: NewReclamationQueue[T](),
		errors:      NewErrorCollector(),
		activeItems: make([]T, 0, opts.MaxBatchSize),
	}

	ba.workers = NewWorkerPool[T](opts.WorkerConcurrency, ba.processBatch)

	goroutinelabels.NewGoroutine("batchaf_aggregator", "main ingest and flusher control loop").StartSimple(func() {
		ba.runLoop()
	})

	return ba, nil
}

// Add blocks until the item is queued or the provided context is canceled.
func (ba *BatchAggregator[T]) Add(ctx context.Context, item T) error {
	if ba.stopped.Load() {
		return ErrAggregatorClosed
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case ba.itemCh <- item:
		atomic.AddUint64(&ba.stats.ItemsSubmitted, 1)
		return nil
	}
}

// Submit performs a non-blocking attempt to queue an item.
func (ba *BatchAggregator[T]) Submit(item T) bool {
	if ba.stopped.Load() {
		return false
	}
	select {
	case ba.itemCh <- item:
		atomic.AddUint64(&ba.stats.ItemsSubmitted, 1)
		return true
	default:
		return false
	}
}

// Push appends multiple items, failing early if context is canceled or aggregator closed.
func (ba *BatchAggregator[T]) Push(ctx context.Context, items ...T) error {
	for _, item := range items {
		if err := ba.Add(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// Flush instructs the aggregator to flush all currently buffered items and waits for dispatch.
func (ba *BatchAggregator[T]) Flush() error {
	if ba.stopped.Load() {
		return ErrAggregatorClosed
	}
	resp := make(chan error, 1)
	ba.flushCh <- resp
	return <-resp
}

// Close gracefully flushes all buffered items, drains the queues, and shuts down workers.
func (ba *BatchAggregator[T]) Close() error {
	if !ba.stopped.CompareAndSwap(false, true) {
		return nil
	}

	close(ba.closeCh)
	ba.sentinel.Stop()

	// Wait for ingest loop to drain itemCh and signal completion
	<-ba.drainDoneCh

	// Stop workers once all batches have been dispatched
	ba.workers.Stop()
	return nil
}

// ReclaimOrphaned inspects in-flight batches exceeding the configured batch timeout.
func (ba *BatchAggregator[T]) ReclaimOrphaned(ctx context.Context) ([]*Batch[T], error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	orphaned := ba.reclamation.SweepOrphaned(ba.opts.BatchTimeout)
	for _, b := range orphaned {
		atomic.AddUint64(&ba.stats.BatchesReclaimed, 1)
		if ba.opts.DeadLetterCallback != nil {
			atomic.AddUint64(&ba.stats.DeadLettered, 1)
			ba.opts.DeadLetterCallback(b, ErrBatchTimeout)
		}
	}
	return orphaned, nil
}

// Stats returns a snapshot of runtime metrics.
func (ba *BatchAggregator[T]) Stats() BatchStats {
	return BatchStats{
		ItemsSubmitted:   atomic.LoadUint64(&ba.stats.ItemsSubmitted),
		BatchesFlushed:   atomic.LoadUint64(&ba.stats.BatchesFlushed),
		BatchesSucceeded: atomic.LoadUint64(&ba.stats.BatchesSucceeded),
		BatchesFailed:    atomic.LoadUint64(&ba.stats.BatchesFailed),
		BatchesRetried:   atomic.LoadUint64(&ba.stats.BatchesRetried),
		BatchesReclaimed: atomic.LoadUint64(&ba.stats.BatchesReclaimed),
		DeadLettered:     atomic.LoadUint64(&ba.stats.DeadLettered),
	}
}

// Errors returns all accumulated worker errors.
func (ba *BatchAggregator[T]) Errors() []error {
	return ba.errors.Errors()
}

// ReclamationQueue returns the internal reclamation queue for testing and monitoring.
func (ba *BatchAggregator[T]) ReclamationQueue() *ReclamationQueue[T] {
	return ba.reclamation
}

// runLoop executes the main select loop handling items, timer ticks, and manual flush signals.
func (ba *BatchAggregator[T]) runLoop() {
	defer close(ba.drainDoneCh)
	for {
		select {
		case item := <-ba.itemCh:
			ba.appendItem(item)
			if len(ba.activeItems) >= ba.opts.MaxBatchSize {
				ba.dispatchCurrentBatch()
			}
		case <-ba.sentinel.Ticks():
			if len(ba.activeItems) > 0 {
				ba.dispatchCurrentBatch()
			}
		case resp := <-ba.flushCh:
			ba.drainPendingItems()
			if len(ba.activeItems) > 0 {
				ba.dispatchCurrentBatch()
			}
			resp <- nil
		case <-ba.closeCh:
			ba.drainRemaining()
			return
		}
	}
}

// drainPendingItems moves any items currently queued in itemCh into activeItems.
func (ba *BatchAggregator[T]) drainPendingItems() {
	for {
		select {
		case item := <-ba.itemCh:
			ba.appendItem(item)
		default:
			return
		}
	}
}

// appendItem safely appends an item to the current active buffer.
func (ba *BatchAggregator[T]) appendItem(item T) {
	ba.mu.Lock()
	defer ba.mu.Unlock()
	ba.activeItems = append(ba.activeItems, item)
}

// dispatchCurrentBatch creates a batch from active items and queues it to workers.
func (ba *BatchAggregator[T]) dispatchCurrentBatch() {
	ba.mu.Lock()
	if len(ba.activeItems) == 0 {
		ba.mu.Unlock()
		return
	}
	items := ba.activeItems
	ba.activeItems = make([]T, 0, ba.opts.MaxBatchSize)
	ba.mu.Unlock()

	seq := ba.batchSeq.Add(1)
	batch := &Batch[T]{
		ID:        fmt.Sprintf("batch-%d-%d", time.Now().UnixNano(), seq),
		Items:     items,
		Status:    BatchStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ba.reclamation.RegisterInFlight(batch)
	atomic.AddUint64(&ba.stats.BatchesFlushed, 1)
	ba.workers.Dispatch(batch)
}

// drainRemaining flushes remaining buffered items during shutdown.
func (ba *BatchAggregator[T]) drainRemaining() {
	// Drain any items already present in itemCh
	for {
		select {
		case item := <-ba.itemCh:
			ba.appendItem(item)
			if len(ba.activeItems) >= ba.opts.MaxBatchSize {
				ba.dispatchCurrentBatch()
			}
		default:
			if len(ba.activeItems) > 0 {
				ba.dispatchCurrentBatch()
			}
			return
		}
	}
}

// processBatch executes flush callbacks with backoff retries and dead-letter routing.
func (ba *BatchAggregator[T]) processBatch(batch *Batch[T]) {
	var lastErr error
	for attempt := 0; attempt <= ba.opts.MaxRetries; attempt++ {
		batch.Attempts = attempt + 1
		ctx, cancel := context.WithTimeout(context.Background(), ba.opts.BatchTimeout)
		err := ba.opts.FlushCallback(ctx, batch.Items)
		cancel()

		if err == nil {
			batch.Status = BatchStatusSucceeded
			ba.reclamation.MarkSuccess(batch.ID)
			atomic.AddUint64(&ba.stats.BatchesSucceeded, 1)
			return
		}

		lastErr = err
		ba.errors.Collect(err)
		atomic.AddUint64(&ba.stats.BatchesRetried, 1)

		if attempt < ba.opts.MaxRetries {
			batch.Status = BatchStatusRetrying
			backoff := ba.calculateBackoff(attempt)
			time.Sleep(backoff)
		}
	}

	// Max retries exceeded
	batch.Status = BatchStatusFailed
	batch.LastError = lastErr
	ba.reclamation.MarkFailed(batch.ID, lastErr)
	atomic.AddUint64(&ba.stats.BatchesFailed, 1)

	if ba.opts.DeadLetterCallback != nil {
		atomic.AddUint64(&ba.stats.DeadLettered, 1)
		ba.opts.DeadLetterCallback(batch, lastErr)
	}
}

// calculateBackoff computes exponential backoff based on attempt index.
func (ba *BatchAggregator[T]) calculateBackoff(attempt int) time.Duration {
	multiplier := 1 << attempt
	if multiplier > 32 {
		multiplier = 32
	}
	return ba.opts.BackoffBase * time.Duration(multiplier)
}
