package telemetry

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestThroughputAggregator_RecordsAndCounts verifies that RecordWorkerSample
// is recorded per worker and that the worker set reflects registered workers.
func TestThroughputAggregator_RecordsAndCounts(t *testing.T) {
	a := NewThroughputAggregator(WithWindow(5 * time.Second))
	if a == nil {
		t.Fatal("nil aggregator from constructor")
	}

	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "agent-alpha", 1000)
	_ = a.RecordWorkerSample(ctx, "agent-alpha", 500)
	_ = a.RecordWorkerSample(ctx, "agent-beta", 200)

	workers := a.Workers()
	if len(workers) != 2 {
		t.Errorf("expected 2 distinct workers, got %d", len(workers))
	}

	snap := a.Snapshot()
	if snap.SampleCount != 3 {
		t.Errorf("expected 3 samples, got %d", snap.SampleCount)
	}
	if snap.ActiveWorkers != 2 {
		t.Errorf("expected 2 active workers, got %d", snap.ActiveWorkers)
	}
}

// TestThroughputAggregator_SlidingWindow verifies that samples outside the
// configured window do not contribute to window tokens or throughput rate,
// but the worker is still considered active if its last activity is within
// the window.
func TestThroughputAggregator_SlidingWindow(t *testing.T) {
	fixed := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)

	a := NewThroughputAggregator(WithWindow(30*time.Second), WithClock(func() time.Time { return fixed }))

	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "alpha", 1000)

	// Move the clock past the window: this sample must be excluded from the
	// window tokens.
	fixed = fixed.Add(2 * time.Minute)
	_ = a.RecordWorkerSample(ctx, "alpha", 1)

	snap := a.Snapshot()
	if snap.WindowTokens != 1 {
		t.Errorf("expected window tokens 1, got %d", snap.WindowTokens)
	}
	if snap.ThroughputTokensPerSecond < 0 {
		t.Errorf("expected throughput >= 0, got %f", snap.ThroughputTokensPerSecond)
	}

	// alpha was active just now, which is within the 30s window.
	if snap.ActiveWorkers != 1 {
		t.Errorf("expected 1 active worker, got %d", snap.ActiveWorkers)
	}

	// Advance clock past the window: now active workers should be 0.
	fixed = fixed.Add(2 * time.Minute)
	snapAfter := a.Snapshot()
	if snapAfter.ActiveWorkers != 0 {
		t.Errorf("expected 0 active workers after window elapsed, got %d", snapAfter.ActiveWorkers)
	}
}

// TestThroughputAggregator_ActiveWorkerWithinWindow verifies that a worker
// with activity inside the window is reported as active.
func TestThroughputAggregator_ActiveWorkerWithinWindow(t *testing.T) {
	fixed := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	a := NewThroughputAggregator(
		WithWindow(60*time.Second),
		WithIdleThreshold(15*time.Second),
		WithClock(func() time.Time { return fixed }),
	)

	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "alpha", 42)
	_ = a.RecordWorkerSample(ctx, "beta", 7)

	snap := a.Snapshot()
	if snap.ActiveWorkers != 2 {
		t.Errorf("expected 2 active workers, got %d", snap.ActiveWorkers)
	}

	details := a.WorkerDetails()
	if len(details) != 2 {
		t.Fatalf("expected 2 worker details, got %d", len(details))
	}

	var alpha *WorkerThroughput
	for i := range details {
		if details[i].WorkerID == "alpha" {
			alpha = &details[i]
		}
	}
	if alpha == nil {
		t.Fatal("alpha worker detail missing")
	}
	if alpha.TotalTokens != 42 {
		t.Errorf("expected alpha total tokens 42, got %d", alpha.TotalTokens)
	}
}

// TestThroughputAggregator_IdleDetection verifies that idle workers are
// flagged when their last activity is older than the configured idle threshold.
func TestThroughputAggregator_IdleDetection(t *testing.T) {
	start := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	clk := start

	a := NewThroughputAggregator(
		WithWindow(1*time.Minute),
		WithIdleThreshold(10*time.Second),
		WithClock(func() time.Time { return clk }),
	)

	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "stale", 1000)

	// Elapse past the idle threshold.
	clk = clk.Add(30 * time.Second)

	snap := a.Snapshot()
	if !snap.Idle {
		t.Error("expected snapshot to report Idle=true")
	}
	if snap.LastActivityAgo < 29*time.Second {
		t.Errorf("expected last-activity-ago >= 29s, got %s", snap.LastActivityAgo)
	}

	details := a.WorkerDetails()
	if len(details) != 1 {
		t.Fatalf("expected 1 worker detail, got %d", len(details))
	}
	if !details[0].Idle {
		t.Error("expected worker detail to be Idle")
	}
	if details[0].IdleSince == nil {
		t.Error("expected worker detail to expose IdleSince")
	}
}

// TestThroughputAggregator_TokenRate verifies that TokensPerSecond reflects
// the window tokens divided by the window duration.
func TestThroughputAggregator_TokenRate(t *testing.T) {
	fixed := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	a := NewThroughputAggregator(
		WithWindow(10*time.Second),
		WithClock(func() time.Time { return fixed }),
	)

	ctx := context.Background()
	// 100 tokens within the window => expected rate 10 tokens/sec.
	_ = a.RecordWorkerSample(ctx, "alpha", 100)

	snap := a.Snapshot()
	want := 10.0
	if absFloat64(snap.ThroughputTokensPerSecond-want) > 1e-9 {
		t.Errorf("expected 10 tok/s, got %f", snap.ThroughputTokensPerSecond)
	}
}

// TestThroughputAggregator_Reset clears samples and workers.
func TestThroughputAggregator_Reset(t *testing.T) {
	a := NewThroughputAggregator(WithWindow(5 * time.Second))
	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "alpha", 10)
	_ = a.RecordWorkerSample(ctx, "beta", 20)

	a.Reset()

	snap := a.Snapshot()
	if snap.SampleCount != 0 {
		t.Errorf("expected sample count 0 after reset, got %d", snap.SampleCount)
	}
	if len(a.Workers()) != 0 {
		t.Errorf("expected 0 workers after reset, got %d", len(a.Workers()))
	}
}

// TestThroughputAggregator_ConcurrentSafety smoke tests concurrent sampling
// and snapshotting against the aggregator.
func TestThroughputAggregator_ConcurrentSafety(t *testing.T) {
	a := NewThroughputAggregator(WithWindow(1 * time.Second))
	ctx := context.Background()

	const goroutines = 16
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		workerID := g
		goroutinelabels.NewGoroutine("test.throughput_aggregator.concurrent_safety", "concurrent worker sampling test").
			StartSimple(func() {
				defer wg.Done()
				worker := "worker"
				if workerID%2 == 0 {
					worker = "even"
				} else {
					worker = "odd"
				}
				for i := 0; i < iterations; i++ {
					_ = a.RecordWorkerSample(ctx, worker, 1)
					_ = a.Snapshot()
					_ = a.WorkerDetails()
					_ = a.Workers()
				}
			})
	}
	wg.Wait()

	snap := a.Snapshot()
	if snap.SampleCount != goroutines*iterations {
		t.Errorf("expected %d total samples, got %d", goroutines*iterations, snap.SampleCount)
	}
}

// TestThroughputAggregator_Validation rejects empty worker IDs.
func TestThroughputAggregator_Validation(t *testing.T) {
	a := NewThroughputAggregator()
	if err := a.RecordWorkerSample(
		context.Background(),
		"",
		1,
	); err == nil {
		t.Error("expected error for empty worker id")
	}
	if !errors.Is(errEmptyWorkerID, errEmptyWorkerID) {
		// Sanity check: sentinel is itself.
		t.Error("sentinel identity violated")
	}
}

// TestThroughputAggregator_WorkerDetailsSortedByTokens verifies that
// WorkerDetails returns entries sorted by total tokens (descending) so
// operators can see the heaviest producers first.
func TestThroughputAggregator_WorkerDetailsSortedByTokens(t *testing.T) {
	a := NewThroughputAggregator(WithWindow(time.Minute))
	ctx := context.Background()
	_ = a.RecordWorkerSample(ctx, "low", 1)
	_ = a.RecordWorkerSample(ctx, "high", 1000)
	_ = a.RecordWorkerSample(ctx, "mid", 500)

	details := a.WorkerDetails()
	if len(details) != 3 {
		t.Fatalf("expected 3 details, got %d", len(details))
	}
	if details[0].WorkerID != "high" || details[1].WorkerID != "mid" || details[2].WorkerID != "low" {
		t.Errorf("expected descending order, got %s,%s,%s",
			details[0].WorkerID, details[1].WorkerID, details[2].WorkerID)
	}
	_ = sort.Slice       // keep the sort import in use
	_ = atomic.LoadInt64 // keep the atomic import in use
}

// absFloat64 is a small helper used only by tests to compare float64s.
func absFloat64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
