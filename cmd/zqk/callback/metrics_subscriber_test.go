package callback

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestMetricsSubscriber_ZeroAllocations(t *testing.T) {
	sub := NewMetricsSubscriber("test_perf")
	entry := &CallbackEntry{
		JobID:     "job-bench-01",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"status": "ok",
		},
	}

	// Warm up
	for i := 0; i < 100; i++ {
		_ = sub.OnEvent(entry)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		_ = sub.OnEvent(entry)
	})

	if allocs > 0 {
		t.Fatalf("expected 0 allocations per OnEvent call on hot path, got %v", allocs)
	}
}

func TestMetricsSubscriber_ConcurrentDispatches(t *testing.T) {
	sub := NewMetricsSubscriber("test_concurrency")
	const numGoroutines = 16
	const eventsPerGoroutine = 5000

	ctx := context.Background()
	budget := goroutinelabels.DefaultBudget()
	pool := goroutinelabels.NewPool(budget, "test_metrics_pool", "concurrent_events", numGoroutines, numGoroutines)
	pool.Start(ctx)
	defer pool.Stop()

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		gid := g
		err := pool.Submit(ctx, func(wCtx context.Context) error {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				entry := &CallbackEntry{
					JobID:     fmt.Sprintf("job-%d-%d", gid, i),
					Timestamp: time.Now(),
				}
				_ = sub.OnEvent(entry)
				if i%100 == 0 {
					sub.RecordDrop(1)
				}
				if i%500 == 0 {
					sub.RecordCircuitBreakerTrip()
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to submit goroutine %d: %v", gid, err)
		}
	}

	wg.Wait()

	snap := sub.Snapshot()
	expectedEvents := uint64(numGoroutines * eventsPerGoroutine)
	if snap.TotalDispatched != expectedEvents {
		t.Fatalf("expected %d total dispatched, got %d", expectedEvents, snap.TotalDispatched)
	}
	if snap.TotalReceived != expectedEvents {
		t.Fatalf("expected %d total received, got %d", expectedEvents, snap.TotalReceived)
	}
	if snap.TotalDropped == 0 {
		t.Fatalf("expected dropped events to be recorded")
	}
	if snap.CircuitBreakerTrips == 0 {
		t.Fatalf("expected circuit breaker trips to be recorded")
	}
}

func TestMetricsSubscriber_LatencyBucketing(t *testing.T) {
	sub := NewMetricsSubscriber("test_latency")

	// Event with known latency: 100 microseconds ago
	t1 := time.Now().Add(-100 * time.Microsecond)
	_ = sub.OnEvent(&CallbackEntry{Timestamp: t1})

	// Event with 1500 microseconds ago
	t2 := time.Now().Add(-1500 * time.Microsecond)
	_ = sub.OnEvent(&CallbackEntry{Timestamp: t2})

	snap := sub.Snapshot()
	if snap.TotalDispatched != 2 {
		t.Fatalf("expected 2 dispatched events, got %d", snap.TotalDispatched)
	}

	// Verify buckets populated
	var totalInBuckets uint64
	for i := 0; i < LatencyBucketCount; i++ {
		totalInBuckets += snap.LatencyBuckets[i]
	}
	if totalInBuckets != 2 {
		t.Fatalf("expected 2 events in latency buckets, got %d", totalInBuckets)
	}
}

func TestMetricsSubscriber_PrometheusExport(t *testing.T) {
	sub := NewMetricsSubscriber("prom_test")
	sub.RecordDrop(42)
	sub.RecordCircuitBreakerTrip()
	_ = sub.OnEvent(&CallbackEntry{
		JobID:     "job-prom-1",
		Timestamp: time.Now().Add(-50 * time.Microsecond),
	})

	output := sub.ExportPrometheus()
	if !strings.Contains(output, "zqk_callback_events_total{subscriber=\"prom_test\",status=\"dispatched\"} 1") {
		t.Fatalf("missing dispatched count in Prometheus output: %s", output)
	}
	if !strings.Contains(output, "zqk_callback_events_total{subscriber=\"prom_test\",status=\"dropped\"} 42") {
		t.Fatalf("missing dropped count in Prometheus output: %s", output)
	}
	if !strings.Contains(output, "zqk_callback_circuit_breaker_trips_total{subscriber=\"prom_test\"} 1") {
		t.Fatalf("missing circuit breaker trips in Prometheus output: %s", output)
	}
	if !strings.Contains(output, "zqk_callback_dispatch_latency_microseconds_bucket") {
		t.Fatalf("missing latency buckets in Prometheus output: %s", output)
	}
}

func TestMetricsSubscriber_Reset(t *testing.T) {
	sub := NewMetricsSubscriber("reset_test")
	_ = sub.OnEvent(&CallbackEntry{JobID: "reset-1"})
	sub.RecordDrop(10)
	sub.RecordCircuitBreakerTrip()

	sub.Reset()
	snap := sub.Snapshot()
	if snap.TotalReceived != 0 || snap.TotalDispatched != 0 || snap.TotalDropped != 0 || snap.CircuitBreakerTrips != 0 {
		t.Fatalf("expected all counters to be 0 after Reset, got %+v", snap)
	}
}
