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
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestOptionsValidation(t *testing.T) {
	t.Parallel()

	validFlush := func(_ context.Context, _ []string) error { return nil }

	baseOpts := DefaultOptions[string]()
	baseOpts.FlushCallback = validFlush

	if err := baseOpts.Validate(); err != nil {
		t.Fatalf("expected valid default options, got: %v", err)
	}

	tests := []struct {
		name    string
		mutate  func(*Options[string])
		wantErr bool
	}{
		{
			name:    "zero MaxBatchSize",
			mutate:  func(o *Options[string]) { o.MaxBatchSize = 0 },
			wantErr: true,
		},
		{
			name:    "negative MaxBatchSize",
			mutate:  func(o *Options[string]) { o.MaxBatchSize = -5 },
			wantErr: true,
		},
		{
			name:    "zero FlushInterval",
			mutate:  func(o *Options[string]) { o.FlushInterval = 0 },
			wantErr: true,
		},
		{
			name:    "zero WorkerConcurrency",
			mutate:  func(o *Options[string]) { o.WorkerConcurrency = 0 },
			wantErr: true,
		},
		{
			name:    "zero BufferCapacity",
			mutate:  func(o *Options[string]) { o.BufferCapacity = 0 },
			wantErr: true,
		},
		{
			name:    "zero BatchTimeout",
			mutate:  func(o *Options[string]) { o.BatchTimeout = 0 },
			wantErr: true,
		},
		{
			name:    "negative MaxRetries",
			mutate:  func(o *Options[string]) { o.MaxRetries = -1 },
			wantErr: true,
		},
		{
			name:    "zero BackoffBase",
			mutate:  func(o *Options[string]) { o.BackoffBase = 0 },
			wantErr: true,
		},
		{
			name:    "nil FlushCallback",
			mutate:  func(o *Options[string]) { o.FlushCallback = nil },
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := baseOpts
			tc.mutate(&opts)
			err := opts.Validate()
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for case %q, got nil", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for case %q: %v", tc.name, err)
			}
		})
	}
}

func TestNewBatchAggregator_InvalidConfig(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions[string]()
	opts.FlushCallback = nil
	agg, err := NewBatchAggregator(opts)
	if err == nil {
		t.Fatal("expected error creating aggregator with nil callback, got nil")
	}
	if agg != nil {
		t.Fatal("expected nil aggregator on invalid config")
	}
}

func TestErrorCollector(t *testing.T) {
	t.Parallel()
	ec := NewErrorCollector()

	if count := ec.Count(); count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}

	ec.Collect(nil)
	if count := ec.Count(); count != 0 {
		t.Fatalf("expected count 0 after collecting nil, got %d", count)
	}

	err1 := errors.New("err 1")
	err2 := errors.New("err 2")
	ec.Collect(err1)
	ec.Collect(err2)

	if count := ec.Count(); count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	errs := ec.Errors()
	if len(errs) != 2 || errs[0] != err1 || errs[1] != err2 {
		t.Fatalf("unexpected errors slice: %v", errs)
	}

	ec.Reset()
	if count := ec.Count(); count != 0 {
		t.Fatalf("expected count 0 after reset, got %d", count)
	}
}

func TestTimerSentinel(t *testing.T) {
	t.Parallel()
	sentinel := NewTimerSentinel(10 * time.Millisecond)

	if interval := sentinel.Interval(); interval != 10*time.Millisecond {
		t.Fatalf("expected interval 10ms, got %v", interval)
	}

	select {
	case <-sentinel.Ticks():
		// Tick received successfully
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for sentinel tick")
	}

	sentinel.Stop()
	// Idempotent stop
	sentinel.Stop()
}

func TestReclamationQueue(t *testing.T) {
	t.Parallel()
	rq := NewReclamationQueue[string]()

	batch1 := &Batch[string]{
		ID:        "batch-1",
		Items:     []string{"item-1"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now().Add(-100 * time.Millisecond),
	}
	rq.RegisterInFlight(batch1)

	if count := rq.InFlightCount(); count != 1 {
		t.Fatalf("expected in-flight count 1, got %d", count)
	}

	got, ok := rq.Get("batch-1")
	if !ok || got.ID != "batch-1" {
		t.Fatalf("failed to retrieve registered batch")
	}

	_, notFound := rq.Get("unknown")
	if notFound {
		t.Fatal("expected not found for unknown batch id")
	}

	// Mark success
	rq.MarkSuccess("batch-1")
	if count := rq.InFlightCount(); count != 0 {
		t.Fatalf("expected in-flight count 0 after success, got %d", count)
	}

	// Mark failed
	batch2 := &Batch[string]{
		ID:        "batch-2",
		Items:     []string{"item-2"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now().Add(-200 * time.Millisecond),
	}
	rq.RegisterInFlight(batch2)
	testErr := errors.New("worker crash")
	rq.MarkFailed("batch-2", testErr)
	if count := rq.InFlightCount(); count != 0 {
		t.Fatalf("expected in-flight count 0 after failure, got %d", count)
	}

	// Sweep orphaned
	batch3 := &Batch[string]{
		ID:        "batch-3",
		Items:     []string{"item-3"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now().Add(-500 * time.Millisecond),
	}
	rq.RegisterInFlight(batch3)

	orphaned := rq.SweepOrphaned(100 * time.Millisecond)
	if len(orphaned) != 1 || orphaned[0].ID != "batch-3" {
		t.Fatalf("expected batch-3 to be orphaned, got: %v", orphaned)
	}

	if count := rq.OrphanedCount(); count != 1 {
		t.Fatalf("expected orphaned count 1, got %d", count)
	}

	allOrphaned := rq.AllOrphaned()
	if len(allOrphaned) != 1 || allOrphaned[0].Status != BatchStatusReclaimed {
		t.Fatalf("expected reclaimed batch status, got: %v", allOrphaned)
	}
}

func TestWorkerPool_Lifecycle(t *testing.T) {
	t.Parallel()
	var processed atomic.Int64

	handler := func(b *Batch[string]) {
		processed.Add(int64(len(b.Items)))
	}

	wp := NewWorkerPool[string](2, handler)

	batch := &Batch[string]{
		ID:    "test-wp-batch",
		Items: []string{"a", "b", "c"},
	}

	dispatched := wp.Dispatch(batch)
	if !dispatched {
		t.Fatal("expected successful dispatch")
	}

	wp.Stop()
	// Idempotent stop
	wp.Stop()

	afterStopDispatched := wp.Dispatch(batch)
	if afterStopDispatched {
		t.Fatal("expected dispatch after stop to return false")
	}

	if val := processed.Load(); val != 3 {
		t.Fatalf("expected 3 processed items, got %d", val)
	}
}

func TestBatchAggregator_CapacityFlush(t *testing.T) {
	t.Parallel()
	var flushedBatches atomic.Int64
	var flushedItems atomic.Int64

	opts := DefaultOptions[string]()
	opts.MaxBatchSize = 5
	opts.FlushInterval = 5 * time.Second
	opts.FlushCallback = func(_ context.Context, batch []string) error {
		flushedBatches.Add(1)
		flushedItems.Add(int64(len(batch)))
		return nil
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		addErr := agg.Add(ctx, fmt.Sprintf("req-%d", i))
		if addErr != nil {
			t.Fatalf("Add failed: %v", addErr)
		}
	}

	// Wait briefly for capacity-triggered flush
	deadline := time.Now().Add(500 * time.Millisecond)
	for flushedBatches.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if flushedBatches.Load() != 1 {
		t.Fatalf("expected 1 capacity flushed batch, got %d", flushedBatches.Load())
	}
	if flushedItems.Load() != 5 {
		t.Fatalf("expected 5 flushed items, got %d", flushedItems.Load())
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
}

func TestBatchAggregator_IntervalFlush(t *testing.T) {
	t.Parallel()
	var flushedBatches atomic.Int64

	opts := DefaultOptions[string]()
	opts.MaxBatchSize = 100
	opts.FlushInterval = 25 * time.Millisecond
	opts.FlushCallback = func(_ context.Context, batch []string) error {
		flushedBatches.Add(1)
		return nil
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	ctx := context.Background()
	if addErr := agg.Add(ctx, "req-interval-1"); addErr != nil {
		t.Fatalf("Add failed: %v", addErr)
	}

	// Wait for interval flush trigger
	deadline := time.Now().Add(500 * time.Millisecond)
	for flushedBatches.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if flushedBatches.Load() == 0 {
		t.Fatal("expected at least 1 interval-triggered flush")
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
}

func TestBatchAggregator_ManualFlushAndPush(t *testing.T) {
	t.Parallel()
	var flushedItems atomic.Int64

	opts := DefaultOptions[string]()
	opts.MaxBatchSize = 500
	opts.FlushInterval = 10 * time.Second
	opts.FlushCallback = func(_ context.Context, batch []string) error {
		flushedItems.Add(int64(len(batch)))
		return nil
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	ctx := context.Background()
	if pushErr := agg.Push(ctx, "item-1", "item-2", "item-3"); pushErr != nil {
		t.Fatalf("Push failed: %v", pushErr)
	}

	if flushErr := agg.Flush(); flushErr != nil {
		t.Fatalf("Flush failed: %v", flushErr)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for flushedItems.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if flushedItems.Load() != 3 {
		t.Fatalf("expected 3 items flushed manually, got %d", flushedItems.Load())
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	// Flush after close returns error
	if flushAfterClose := agg.Flush(); flushAfterClose == nil {
		t.Fatal("expected error on Flush() after Close(), got nil")
	}
}

func TestBatchAggregator_SubmitAndBufferLimits(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions[string]()
	opts.BufferCapacity = 2
	opts.MaxBatchSize = 100
	opts.FlushInterval = 10 * time.Second
	opts.FlushCallback = func(_ context.Context, _ []string) error {
		return nil
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	// Submit should succeed while buffer has capacity
	ok1 := agg.Submit("submit-1")
	ok2 := agg.Submit("submit-2")

	if !ok1 || !ok2 {
		t.Fatalf("expected Submit to succeed, got ok1=%v, ok2=%v", ok1, ok2)
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	// Submit and Add after Close
	if agg.Submit("after-close") {
		t.Fatal("expected Submit to return false after Close")
	}

	ctx := context.Background()
	if addErr := agg.Add(ctx, "after-close"); !errors.Is(addErr, ErrAggregatorClosed) {
		t.Fatalf("expected ErrAggregatorClosed, got: %v", addErr)
	}
}

func TestBatchAggregator_AddContextCancellation(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions[string]()
	opts.BufferCapacity = 1
	opts.MaxBatchSize = 1000
	opts.FlushInterval = 10 * time.Second
	opts.FlushCallback = func(_ context.Context, _ []string) error { return nil }

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}
	defer func() {
		if cErr := agg.Close(); cErr != nil {
			t.Fatalf("defer Close failed: %v", cErr)
		}
	}()

	// Fill buffer
	agg.appendItem("buffered-pre-fill")
	agg.Submit("fill-1")

	// Now try Add with already-canceled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()

	errCanceled := agg.Add(ctxCancel, "blocked-item")
	if !errors.Is(errCanceled, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", errCanceled)
	}

	// Test Push with canceled context
	pushErr := agg.Push(ctxCancel, "blocked-push")
	if !errors.Is(pushErr, context.Canceled) {
		t.Fatalf("expected context.Canceled for Push, got: %v", pushErr)
	}
}

func TestBatchAggregator_RetryAndDeadLetter(t *testing.T) {
	t.Parallel()
	var attempts atomic.Int64
	var deadLetteredBatch *Batch[string]
	var deadLetterErr error
	var dlMu sync.Mutex

	opts := DefaultOptions[string]()
	opts.MaxBatchSize = 2
	opts.MaxRetries = 2
	opts.BackoffBase = 5 * time.Millisecond
	opts.FlushInterval = 10 * time.Second
	opts.FlushCallback = func(_ context.Context, _ []string) error {
		attempts.Add(1)
		return errors.New("simulated transient network failure")
	}
	opts.DeadLetterCallback = func(b *Batch[string], err error) {
		dlMu.Lock()
		defer dlMu.Unlock()
		deadLetteredBatch = b
		deadLetterErr = err
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	ctx := context.Background()
	if addErr := agg.Add(ctx, "req-fail-1"); addErr != nil {
		t.Fatalf("Add failed: %v", addErr)
	}
	if addErr := agg.Add(ctx, "req-fail-2"); addErr != nil {
		t.Fatalf("Add failed: %v", addErr)
	}

	// Await exhaustion of attempts: 1 initial + 2 retries = 3 attempts
	deadline := time.Now().Add(1 * time.Second)
	for attempts.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if attempts.Load() < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", attempts.Load())
	}

	dlMu.Lock()
	dlBatch := deadLetteredBatch
	dlErr := deadLetterErr
	dlMu.Unlock()

	if dlBatch == nil || dlErr == nil {
		t.Fatal("expected dead letter callback to be invoked with failed batch")
	}

	if dlBatch.Status != BatchStatusFailed {
		t.Fatalf("expected batch status Failed, got: %v", dlBatch.Status)
	}

	stats := agg.Stats()
	if stats.BatchesFailed == 0 {
		t.Fatal("expected BatchesFailed > 0 in stats")
	}
	if stats.DeadLettered == 0 {
		t.Fatal("expected DeadLettered > 0 in stats")
	}

	errs := agg.Errors()
	if len(errs) == 0 {
		t.Fatal("expected errors collected in aggregator")
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
}

func TestBatchAggregator_OrphanedReclamation(t *testing.T) {
	t.Parallel()
	var dlCalled atomic.Bool

	opts := DefaultOptions[string]()
	opts.BatchTimeout = 20 * time.Millisecond
	opts.FlushCallback = func(_ context.Context, _ []string) error { return nil }
	opts.DeadLetterCallback = func(_ *Batch[string], _ error) {
		dlCalled.Store(true)
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	// Register an artificial in-flight batch directly into aggregator's reclamation queue
	rq := agg.ReclamationQueue()
	fakeBatch := &Batch[string]{
		ID:        "orphaned-batch-101",
		Items:     []string{"orphaned-req-a"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now().Add(-50 * time.Millisecond),
	}
	rq.RegisterInFlight(fakeBatch)

	// Reclaim with canceled context
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, errCanc := agg.ReclaimOrphaned(cancCtx)
	if !errors.Is(errCanc, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", errCanc)
	}

	// Reclaim with valid context
	reclaimed, reclaimErr := agg.ReclaimOrphaned(context.Background())
	if reclaimErr != nil {
		t.Fatalf("ReclaimOrphaned failed: %v", reclaimErr)
	}

	if len(reclaimed) != 1 || reclaimed[0].ID != "orphaned-batch-101" {
		t.Fatalf("unexpected reclaimed batches: %v", reclaimed)
	}

	if !dlCalled.Load() {
		t.Fatal("expected DeadLetterCallback to be called on reclaimed orphaned batch")
	}

	stats := agg.Stats()
	if stats.BatchesReclaimed != 1 {
		t.Fatalf("expected BatchesReclaimed=1, got %d", stats.BatchesReclaimed)
	}

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}
	// Idempotent close
	if closeAgainErr := agg.Close(); closeAgainErr != nil {
		t.Fatalf("second Close failed: %v", closeAgainErr)
	}
}

func TestBatchAggregator_HighConcurrencyProducers(t *testing.T) {
	t.Parallel()
	const numGoroutines = 8
	const itemsPerGoroutine = 50

	var totalFlushed atomic.Int64

	opts := DefaultOptions[string]()
	opts.MaxBatchSize = 25
	opts.FlushInterval = 20 * time.Millisecond
	opts.WorkerConcurrency = 4
	opts.BufferCapacity = numGoroutines * itemsPerGoroutine * 2
	opts.FlushCallback = func(_ context.Context, batch []string) error {
		totalFlushed.Add(int64(len(batch)))
		return nil
	}

	agg, err := NewBatchAggregator(opts)
	if err != nil {
		t.Fatalf("failed to create aggregator: %v", err)
	}

	var wg sync.WaitGroup
	ctx := context.Background()

	for g := 0; g < numGoroutines; g++ {
		gid := g
		wg.Add(1)
		builder := goroutinelabels.NewGoroutine("batchaf_test_producer", fmt.Sprintf("producer %d", gid))
		builder.StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerGoroutine; i++ {
				item := fmt.Sprintf("goroutine-%d-item-%d", gid, i)
				addErr := agg.Add(ctx, item)
				if addErr != nil {
					t.Errorf("Add failed: %v", addErr)
				}
			}
		})
	}

	wg.Wait()

	if closeErr := agg.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	expectedTotal := int64(numGoroutines * itemsPerGoroutine)
	if totalFlushed.Load() != expectedTotal {
		t.Fatalf("expected %d total items flushed, got %d", expectedTotal, totalFlushed.Load())
	}
}

func TestProcessOrphanedRequirements_ComprehensiveBranches(t *testing.T) {
	t.Parallel()

	// Nil context with empty slice
	if err := ProcessOrphanedRequirements(nil, nil); err != nil {
		t.Fatalf("expected nil for nil ctx and empty reqs, got: %v", err)
	}

	// Nil context with valid slice
	reqs := []string{"REQ-CYC-1", "REQ-CYC-2"}
	if err := ProcessOrphanedRequirements(nil, reqs); err != nil {
		t.Fatalf("expected nil for valid reqs with nil ctx, got: %v", err)
	}

	// Cancel context during processing
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ProcessOrphanedRequirements(ctx, []string{"REQ-1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	// Parse identity error
	if err := ProcessOrphanedRequirements(context.Background(), []string{""}); err == nil {
		t.Fatal("expected error on empty requirement identity, got nil")
	}

	// Governance error
	if err := ProcessOrphanedRequirements(context.Background(), []string{"invalid-governance"}); err == nil {
		t.Fatal("expected error on invalid-governance, got nil")
	}

	// Translation error
	if err := ProcessOrphanedRequirements(context.Background(), []string{"invalid-translation"}); err == nil {
		t.Fatal("expected error on invalid-translation, got nil")
	}
}
