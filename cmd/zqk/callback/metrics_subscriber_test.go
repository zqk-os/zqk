package callback

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func assertSnapshotCounters(t *testing.T, snap MetricsSnapshot, expectedDispatched, expectedErrors, expectedRetries int64) {
	t.Helper()
	require.Equal(t, expectedDispatched, snap.TotalDispatched, "total dispatched mismatch")
	require.Equal(t, expectedErrors, snap.TotalErrors, "total errors mismatch")
	require.Equal(t, expectedRetries, snap.TotalRetries, "total retries mismatch")
}

func assertPercentileOrdering(t *testing.T, snap MetricsSnapshot) {
	t.Helper()
	require.GreaterOrEqual(t, snap.P50, time.Duration(0), "p50 must be non-negative")
	require.GreaterOrEqual(t, snap.P95, snap.P50, "p95 must be >= p50")
	require.GreaterOrEqual(t, snap.P99, snap.P95, "p99 must be >= p95")
}

func assertPrometheusContains(t *testing.T, output, expected string) {
	t.Helper()
	require.True(t, strings.Contains(output, expected), "prometheus output missing expected fragment: %s", expected)
}

func TestMetricsSubscriber_ZeroAllocations(t *testing.T) {
	sub := NewMetricsSubscriber("test_perf")
	entry := &CallbackEntry{
		JobID:     "job-bench-01",
		Timestamp: time.Now(),
		Payload: map[string]any{
			"status": "ok",
		},
	}
	elapsed := 50 * time.Microsecond

	for i := 0; i < 100; i++ {
		sub.DispatchTelemetryFast(entry, elapsed)
		sub.RecordLatency(elapsed)
	}

	allocsFast := testing.AllocsPerRun(1000, func() {
		sub.DispatchTelemetryFast(entry, elapsed)
	})
	require.Equal(t, float64(0), allocsFast, "expected 0 allocations on DispatchTelemetryFast")

	allocsLatency := testing.AllocsPerRun(1000, func() {
		sub.RecordLatency(elapsed)
	})
	require.Equal(t, float64(0), allocsLatency, "expected 0 allocations on RecordLatency")
}

func TestMetricsSubscriber_CounterIncrementsAndErrorTracking(t *testing.T) {
	sub := NewMetricsSubscriber("test_counters")
	ctx := context.Background()

	validEntry := &CallbackEntry{
		JobID:     "job-1",
		Timestamp: time.Now().Add(-10 * time.Millisecond),
	}
	errValid := sub.Notify(ctx, validEntry)
	require.NoError(t, errValid)

	errNil := sub.Notify(ctx, nil)
	require.NoError(t, errNil)

	errorEntry := &CallbackEntry{
		JobID: "job-err",
		Payload: map[string]any{
			"status": "failed",
			"error":  "timeout exceeded",
			"retry":  "true",
		},
	}
	errError := sub.Notify(ctx, errorEntry)
	require.NoError(t, errError)

	sub.RecordDrop(5)
	sub.RecordCircuitBreakerTrip()

	snap := sub.Snapshot()
	assertSnapshotCounters(t, snap, 3, 2, 1)
	require.Equal(t, int64(5), snap.TotalDropped)
	require.Equal(t, int64(1), snap.CircuitBreakerTrips)
	require.Equal(t, "test_counters", sub.Name())

	sub.Reset()
	resetSnap := sub.Snapshot()
	assertSnapshotCounters(t, resetSnap, 0, 0, 0)
	require.Equal(t, int64(0), resetSnap.TotalDropped)
	require.Equal(t, int64(0), resetSnap.CircuitBreakerTrips)
}

func TestMetricsSubscriber_LatencyPercentiles(t *testing.T) {
	sub := NewMetricsSubscriber("test_latency")

	emptySnap := sub.Snapshot()
	require.Equal(t, time.Duration(0), emptySnap.P50)
	require.Equal(t, time.Duration(0), emptySnap.P95)
	require.Equal(t, time.Duration(0), emptySnap.P99)

	sub.RecordLatency(0)
	sub.RecordLatency(-5 * time.Millisecond)

	for i := 0; i < 50; i++ {
		sub.RecordLatency(100 * time.Microsecond)
	}
	for i := 0; i < 45; i++ {
		sub.RecordLatency(1 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		sub.RecordLatency(20 * time.Millisecond)
	}

	snap := sub.Snapshot()
	assertPercentileOrdering(t, snap)

	require.GreaterOrEqual(t, snap.P50, 50*time.Microsecond)
	require.LessOrEqual(t, snap.P50, 2*time.Millisecond)
	require.GreaterOrEqual(t, snap.P95, 500*time.Microsecond)
	require.GreaterOrEqual(t, snap.P99, 5*time.Millisecond)
}

func TestMetricsSubscriber_RingBufferTelemetry(t *testing.T) {
	sub := NewMetricsSubscriber("test_ring_buffer")

	sub.RecordRingBuffer(12, 1024, 3)
	snap := sub.Snapshot()
	require.Equal(t, int64(12), snap.RingBufferSize)
	require.Equal(t, int64(1024), snap.RingBufferCapacity)
	require.Equal(t, int64(3), snap.RingBufferEvictions)

	rb := NewRingBuffer(64)
	rb.Push(&CallbackEntry{JobID: "rb-1", Seq: 1})
	rb.Push(&CallbackEntry{JobID: "rb-2", Seq: 2})
	sub.AttachRingBuffer(rb)

	snapRB := sub.Snapshot()
	require.Equal(t, int64(2), snapRB.RingBufferSize)
	require.Equal(t, int64(64), snapRB.RingBufferCapacity)
	require.Equal(t, int64(0), snapRB.RingBufferEvictions)

	srb := NewShardedRingBuffer(4, 32)
	srb.Push(&CallbackEntry{JobID: "srb-1", Seq: 1})
	sub.AttachShardedRingBuffer(srb)

	snapSRB := sub.Snapshot()
	require.Equal(t, int64(1), snapSRB.RingBufferSize)
	require.Equal(t, int64(4*32), snapSRB.RingBufferCapacity)

	sub.AttachRingBuffer(nil)
	sub.AttachShardedRingBuffer(nil)
}

func TestMetricsSubscriber_ConcurrentDispatches(t *testing.T) {
	sub := NewMetricsSubscriber("test_concurrent")
	const numWorkers = 8
	const eventsPerWorker = 500

	ctx := context.Background()
	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		wid := w
		goroutinelabels.NewGoroutine("test_concurrent_metrics", "concurrent metrics dispatch").StartSimple(func() {
			dispatchConcurrentBatch(ctx, sub, wid, eventsPerWorker, &wg, errCh)
		})
	}

	wg.Wait()
	close(errCh)

	for dispatchErr := range errCh {
		require.NoError(t, dispatchErr)
	}

	snap := sub.Snapshot()
	expectedTotal := int64(numWorkers * eventsPerWorker)
	require.Equal(t, expectedTotal, snap.TotalDispatched)
	require.Equal(t, expectedTotal, snap.TotalReceived)
	assertPercentileOrdering(t, snap)
}

func dispatchConcurrentBatch(ctx context.Context, sub *MetricsSubscriber, wid, count int, wg *sync.WaitGroup, errCh chan error) {
	defer wg.Done()
	for i := 0; i < count; i++ {
		entry := &CallbackEntry{
			JobID:     fmt.Sprintf("worker-%d-job-%d", wid, i),
			Timestamp: time.Now().Add(-100 * time.Microsecond),
		}
		if notifyErr := sub.Notify(ctx, entry); notifyErr != nil {
			errCh <- notifyErr
			return
		}
		if i%50 == 0 {
			sub.RecordDrop(1)
		}
		if i%100 == 0 {
			sub.RecordCircuitBreakerTrip()
		}
	}
}

func TestMetricsSubscriber_PrometheusExport(t *testing.T) {
	sub := NewMetricsSubscriber("prom_exporter")
	ctx := context.Background()

	notifyErr := sub.Notify(ctx, &CallbackEntry{
		JobID:     "prom-1",
		Timestamp: time.Now().Add(-200 * time.Microsecond),
	})
	require.NoError(t, notifyErr)

	sub.RecordDrop(3)
	sub.RecordCircuitBreakerTrip()
	sub.RecordRingBuffer(5, 512, 1)

	output := sub.ExportPrometheus()
	assertPrometheusContains(t, output, `zqk_callback_events_total{subscriber="prom_exporter",status="dispatched"} 1`)
	assertPrometheusContains(t, output, `zqk_callback_events_total{subscriber="prom_exporter",status="dropped"} 3`)
	assertPrometheusContains(t, output, `zqk_callback_circuit_breaker_trips_total{subscriber="prom_exporter"} 1`)
	assertPrometheusContains(t, output, `zqk_callback_ring_buffer_size{subscriber="prom_exporter"} 5`)
	assertPrometheusContains(t, output, `zqk_callback_ring_buffer_capacity{subscriber="prom_exporter"} 512`)
	assertPrometheusContains(t, output, `zqk_callback_dispatch_latency_nanoseconds_bucket`)
}

func TestMetricsSubscriber_ContextCancellation(t *testing.T) {
	sub := NewMetricsSubscriber("test_ctx_cancel")
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := sub.Notify(canceledCtx, &CallbackEntry{JobID: "canceled-job"})
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)

	snap := sub.Snapshot()
	require.Equal(t, int64(1), snap.TotalErrors)
}
