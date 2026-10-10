package callback

import (
	"context"
	"fmt"
	"math/bits"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// DefaultMetricsSubscriberName is the default registration name for the metrics subscriber.
const DefaultMetricsSubscriberName = "metrics_subscriber"

// LatencyBucketCount defines the number of power-of-two nanosecond latency buckets.
// Bucket 0: 0ns / non-positive duration.
// Bucket 1: 1ns.
// Bucket 2..63: [2^(k-1), 2^k - 1] ns.
const LatencyBucketCount = 64

var (
	_ CallbackSubscriber = (*MetricsSubscriber)(nil)
	_ Subscriber         = (*MetricsSubscriber)(nil)
)

// MetricsSnapshot represents an immutable point-in-time snapshot of telemetry metrics.
type MetricsSnapshot struct {
	Name                string
	TotalDispatched     int64
	TotalErrors         int64
	TotalRetries        int64
	TotalReceived       int64
	TotalDropped        int64
	CircuitBreakerTrips int64
	ActiveInFlight      int64
	P50                 time.Duration
	P95                 time.Duration
	P99                 time.Duration
	RingBufferSize      int64
	RingBufferCapacity  int64
	RingBufferEvictions int64
	LatencyBuckets      [LatencyBucketCount]int64
}

// MetricsSubscriber aggregates event dispatch counters, ring buffer telemetry,
// and zero-allocation latency distributions across the callback bus.
type MetricsSubscriber struct {
	name                string
	totalDispatched     atomic.Int64
	totalErrors         atomic.Int64
	totalRetries        atomic.Int64
	totalReceived       atomic.Int64
	totalDropped        atomic.Int64
	circuitBreakerTrips atomic.Int64
	activeInFlight      atomic.Int64
	ringBufferSize      atomic.Int64
	ringBufferCapacity  atomic.Int64
	ringBufferEvictions atomic.Int64
	latencyBuckets      [LatencyBucketCount]atomic.Int64
}

// NewMetricsSubscriber initializes a MetricsSubscriber with default or customized name.
func NewMetricsSubscriber(names ...string) *MetricsSubscriber {
	name := DefaultMetricsSubscriberName
	if len(names) > 0 && names[0] != "" {
		name = names[0]
	}
	return &MetricsSubscriber{
		name: name,
	}
}

// Name returns the subscriber identifier.
func (m *MetricsSubscriber) Name() string {
	return m.name
}

// Notify handles incoming callback entries, tracking in-flight work and recording metrics.
func (m *MetricsSubscriber) Notify(ctx context.Context, entry *CallbackEntry) error {
	if ctx != nil && ctx.Err() != nil {
		m.totalErrors.Add(1)
		return ctx.Err()
	}

	m.activeInFlight.Add(1)
	defer m.activeInFlight.Add(-1)

	m.totalReceived.Add(1)

	var elapsed time.Duration
	if entry != nil && !entry.Timestamp.IsZero() {
		elapsed = time.Since(entry.Timestamp)
	}

	m.DispatchTelemetryFast(entry, elapsed)
	return nil
}

// DispatchTelemetryFast is a lock-free, zero-allocation hook for event telemetry dispatch.
func (m *MetricsSubscriber) DispatchTelemetryFast(entry *CallbackEntry, elapsed time.Duration) {
	m.totalDispatched.Add(1)

	if elapsed > 0 {
		m.RecordLatency(elapsed)
	}

	if entry == nil {
		m.totalErrors.Add(1)
		return
	}

	if entry.Payload != nil {
		m.extractPayloadMetrics(entry.Payload)
	}
}

func (m *MetricsSubscriber) extractPayloadMetrics(payload map[string]any) {
	status := objects.GetString(payload, "status")
	isErrorStatus := status == "error" || status == "failed" || status == "failure"
	hasErrorMsg := objects.GetString(payload, "error") != ""
	if isErrorStatus || hasErrorMsg {
		m.totalErrors.Add(1)
	}
	if objects.GetString(payload, "retry") != "" || objects.GetString(payload, "retries") != "" {
		m.totalRetries.Add(1)
	}
}

// RecordLatency records an observed duration into power-of-two nanosecond distribution buckets.
func (m *MetricsSubscriber) RecordLatency(d time.Duration) {
	if d <= 0 {
		m.latencyBuckets[0].Add(1)
		return
	}

	ns := d.Nanoseconds()
	bucket := bits.Len64(uint64(ns))
	if bucket >= LatencyBucketCount {
		bucket = LatencyBucketCount - 1
	}
	m.latencyBuckets[bucket].Add(1)
}

// RecordError increments the error counter.
func (m *MetricsSubscriber) RecordError() {
	m.totalErrors.Add(1)
}

// RecordRetry increments the retry counter.
func (m *MetricsSubscriber) RecordRetry() {
	m.totalRetries.Add(1)
}

// RecordDrop increments dropped events by count.
func (m *MetricsSubscriber) RecordDrop(count int64) {
	if count > 0 {
		m.totalDropped.Add(count)
	}
}

// RecordCircuitBreakerTrip increments the circuit breaker trips counter.
func (m *MetricsSubscriber) RecordCircuitBreakerTrip() {
	m.circuitBreakerTrips.Add(1)
}

// RecordRingBuffer records point-in-time ring buffer statistics.
func (m *MetricsSubscriber) RecordRingBuffer(size, capacity, evicted int64) {
	m.ringBufferSize.Store(size)
	m.ringBufferCapacity.Store(capacity)
	m.ringBufferEvictions.Store(evicted)
}

// AttachRingBuffer samples telemetry directly from a single RingBuffer instance.
func (m *MetricsSubscriber) AttachRingBuffer(rb *RingBuffer) {
	if rb == nil {
		return
	}
	m.RecordRingBuffer(int64(rb.Size()), int64(rb.Capacity()), rb.EvictedCount())
}

// AttachShardedRingBuffer samples telemetry directly from a ShardedRingBuffer instance.
func (m *MetricsSubscriber) AttachShardedRingBuffer(srb *ShardedRingBuffer) {
	if srb == nil {
		return
	}
	m.RecordRingBuffer(int64(srb.TotalSize()), int64(srb.TotalCapacity()), srb.EvictedCount())
}

// Snapshot captures a consistent point-in-time view of all metrics.
func (m *MetricsSubscriber) Snapshot() MetricsSnapshot {
	snap := MetricsSnapshot{
		Name:                m.name,
		TotalDispatched:     m.totalDispatched.Load(),
		TotalErrors:         m.totalErrors.Load(),
		TotalRetries:        m.totalRetries.Load(),
		TotalReceived:       m.totalReceived.Load(),
		TotalDropped:        m.totalDropped.Load(),
		CircuitBreakerTrips: m.circuitBreakerTrips.Load(),
		ActiveInFlight:      m.activeInFlight.Load(),
		RingBufferSize:      m.ringBufferSize.Load(),
		RingBufferCapacity:  m.ringBufferCapacity.Load(),
		RingBufferEvictions: m.ringBufferEvictions.Load(),
	}

	var totalSamples int64
	for i := 0; i < LatencyBucketCount; i++ {
		count := m.latencyBuckets[i].Load()
		snap.LatencyBuckets[i] = count
		totalSamples += count
	}

	snap.P50 = calculatePercentile(snap.LatencyBuckets, totalSamples, 0.50)
	snap.P95 = calculatePercentile(snap.LatencyBuckets, totalSamples, 0.95)
	snap.P99 = calculatePercentile(snap.LatencyBuckets, totalSamples, 0.99)

	return snap
}

func calculatePercentile(buckets [LatencyBucketCount]int64, totalSamples int64, p float64) time.Duration {
	if totalSamples <= 0 || p <= 0 {
		return 0
	}
	target := int64(float64(totalSamples) * p)
	if target < 1 {
		target = 1
	}
	if target > totalSamples {
		target = totalSamples
	}

	var accumulated int64
	for b := 0; b < LatencyBucketCount; b++ {
		count := buckets[b]
		if count <= 0 {
			continue
		}
		accumulated += count
		if accumulated >= target {
			rankInBucket := target - (accumulated - count)
			return bucketToDuration(b, count, rankInBucket)
		}
	}
	return bucketToDuration(LatencyBucketCount-1, 1, 1)
}

func bucketToDuration(bucket int, bucketCount, rankInBucket int64) time.Duration {
	if bucket <= 0 {
		return 0
	}
	if bucket == 1 {
		return time.Nanosecond
	}
	lower := int64(1) << (bucket - 1)
	upper := (int64(1) << bucket) - 1
	if bucketCount <= 1 || rankInBucket <= 1 {
		return time.Duration(lower) * time.Nanosecond
	}
	fraction := float64(rankInBucket-1) / float64(bucketCount)
	interpolated := float64(lower) + fraction*float64(upper-lower)
	return time.Duration(interpolated) * time.Nanosecond
}

// Reset zeroes all internal metrics and latency distribution buckets.
func (m *MetricsSubscriber) Reset() {
	m.totalDispatched.Store(0)
	m.totalErrors.Store(0)
	m.totalRetries.Store(0)
	m.totalReceived.Store(0)
	m.totalDropped.Store(0)
	m.circuitBreakerTrips.Store(0)
	m.activeInFlight.Store(0)
	m.ringBufferSize.Store(0)
	m.ringBufferCapacity.Store(0)
	m.ringBufferEvictions.Store(0)
	for i := 0; i < LatencyBucketCount; i++ {
		m.latencyBuckets[i].Store(0)
	}
}

// ExportPrometheus formats the metrics into standard Prometheus text exposition format.
func (m *MetricsSubscriber) ExportPrometheus() string {
	s := m.Snapshot()
	var b strings.Builder

	fmt.Fprintf(&b, "# HELP zqk_callback_events_total Total callback events processed\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_events_total counter\n")
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"received\"} %d\n", s.Name, s.TotalReceived)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"dispatched\"} %d\n", s.Name, s.TotalDispatched)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"errors\"} %d\n", s.Name, s.TotalErrors)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"retries\"} %d\n", s.Name, s.TotalRetries)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"dropped\"} %d\n", s.Name, s.TotalDropped)

	fmt.Fprintf(&b, "# HELP zqk_callback_active_in_flight Active dispatches in flight\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_active_in_flight gauge\n")
	fmt.Fprintf(&b, "zqk_callback_active_in_flight{subscriber=%q} %d\n", s.Name, s.ActiveInFlight)

	fmt.Fprintf(&b, "# HELP zqk_callback_circuit_breaker_trips_total Total circuit breaker trips\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_circuit_breaker_trips_total counter\n")
	fmt.Fprintf(&b, "zqk_callback_circuit_breaker_trips_total{subscriber=%q} %d\n", s.Name, s.CircuitBreakerTrips)

	fmt.Fprintf(&b, "# HELP zqk_callback_ring_buffer_size Current ring buffer queue size\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_ring_buffer_size gauge\n")
	fmt.Fprintf(&b, "zqk_callback_ring_buffer_size{subscriber=%q} %d\n", s.Name, s.RingBufferSize)

	fmt.Fprintf(&b, "# HELP zqk_callback_ring_buffer_capacity Ring buffer maximum capacity\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_ring_buffer_capacity gauge\n")
	fmt.Fprintf(&b, "zqk_callback_ring_buffer_capacity{subscriber=%q} %d\n", s.Name, s.RingBufferCapacity)

	fmt.Fprintf(&b, "# HELP zqk_callback_ring_buffer_evictions_total Ring buffer evictions count\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_ring_buffer_evictions_total counter\n")
	fmt.Fprintf(&b, "zqk_callback_ring_buffer_evictions_total{subscriber=%q} %d\n", s.Name, s.RingBufferEvictions)

	m.writePrometheusLatency(&b, &s)
	return b.String()
}

func (m *MetricsSubscriber) writePrometheusLatency(b *strings.Builder, s *MetricsSnapshot) {
	fmt.Fprintf(b, "# HELP zqk_callback_dispatch_latency_nanoseconds Latency summary percentiles\n")
	fmt.Fprintf(b, "# TYPE zqk_callback_dispatch_latency_nanoseconds summary\n")
	fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds{subscriber=%q,quantile=\"0.50\"} %d\n", s.Name, s.P50.Nanoseconds())
	fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds{subscriber=%q,quantile=\"0.95\"} %d\n", s.Name, s.P95.Nanoseconds())
	fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds{subscriber=%q,quantile=\"0.99\"} %d\n", s.Name, s.P99.Nanoseconds())

	fmt.Fprintf(b, "# HELP zqk_callback_dispatch_latency_nanoseconds_bucket Power-of-two latency histogram\n")
	fmt.Fprintf(b, "# TYPE zqk_callback_dispatch_latency_nanoseconds_bucket histogram\n")
	var cumulative int64
	for i := 0; i < LatencyBucketCount; i++ {
		cumulative += s.LatencyBuckets[i]
		if i == 0 {
			fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds_bucket{subscriber=%q,le=\"0\"} %d\n", s.Name, cumulative)
		} else if i == LatencyBucketCount-1 {
			fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds_bucket{subscriber=%q,le=\"+Inf\"} %d\n", s.Name, cumulative)
		} else {
			le := int64(1) << i
			fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds_bucket{subscriber=%q,le=\"%d\"} %d\n", s.Name, le, cumulative)
		}
	}
	fmt.Fprintf(b, "zqk_callback_dispatch_latency_nanoseconds_count{subscriber=%q} %d\n", s.Name, cumulative)
}
