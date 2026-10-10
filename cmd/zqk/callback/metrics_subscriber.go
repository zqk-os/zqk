// Package callback provides resilient, event-driven callback mechanisms,
// sharded ring buffering, backpressure circuit breaking, and telemetry aggregation.
package callback

import (
	"fmt"
	"math/bits"
	"strings"
	"sync/atomic"
	"time"
)

// LatencyBucketCount defines the number of power-of-two microsecond latency buckets.
// Bucket 0: <1us, Bucket 1: <2us, Bucket 2: <4us, ..., Bucket 15: >=16384us (~16.4ms)
const LatencyBucketCount = 16

// MetricsSnapshot is a thread-safe, point-in-time snapshot of telemetry metrics.
type MetricsSnapshot struct {
	Name                 string
	TotalReceived        uint64
	TotalDispatched      uint64
	TotalDropped         uint64
	TotalFailures        uint64
	CircuitBreakerTrips  uint64
	LastEventTimestampNs int64
	LatencyBuckets       [LatencyBucketCount]uint64
}

// MetricsSubscriber collects high-frequency event bus metrics with zero heap allocations on the hot path.
type MetricsSubscriber struct {
	name                 string
	totalReceived        uint64
	totalDispatched      uint64
	totalDropped         uint64
	totalFailures        uint64
	circuitBreakerTrips  uint64
	lastEventTimestampNs int64
	latencyBuckets       [LatencyBucketCount]uint64
}

// NewMetricsSubscriber creates a new zero-allocation MetricsSubscriber.
func NewMetricsSubscriber(name string) *MetricsSubscriber {
	if name == "" {
		name = "telemetry_metrics_subscriber"
	}
	return &MetricsSubscriber{
		name: name,
	}
}

// Name returns the unique subscriber name for dispatcher registration.
func (m *MetricsSubscriber) Name() string {
	return m.name
}

// OnEvent processes an incoming CallbackEntry. It executes on the hot path with 0 heap allocations.
func (m *MetricsSubscriber) OnEvent(entry *CallbackEntry) error {
	nowNs := time.Now().UnixNano()
	atomic.StoreInt64(&m.lastEventTimestampNs, nowNs)
	atomic.AddUint64(&m.totalReceived, 1)

	if entry == nil {
		atomic.AddUint64(&m.totalFailures, 1)
		return nil
	}

	// Calculate latency if Timestamp is available
	if !entry.Timestamp.IsZero() {
		latencyUs := uint64(time.Since(entry.Timestamp).Microseconds())
		bucket := latencyToBucket(latencyUs)
		atomic.AddUint64(&m.latencyBuckets[bucket], 1)
	}

	atomic.AddUint64(&m.totalDispatched, 1)
	return nil
}

// RecordDrop increments the dropped events counter with 0 allocations.
func (m *MetricsSubscriber) RecordDrop(count uint64) {
	if count > 0 {
		atomic.AddUint64(&m.totalDropped, count)
	}
}

// RecordCircuitBreakerTrip increments the circuit breaker trip counter.
func (m *MetricsSubscriber) RecordCircuitBreakerTrip() {
	atomic.AddUint64(&m.circuitBreakerTrips, 1)
}

// RecordFailure increments the dispatch failure counter.
func (m *MetricsSubscriber) RecordFailure() {
	atomic.AddUint64(&m.totalFailures, 1)
}

// latencyToBucket maps a microsecond latency value into a power-of-two histogram bucket (0 to LatencyBucketCount-1).
func latencyToBucket(latencyUs uint64) int {
	if latencyUs == 0 {
		return 0
	}
	// bits.Len64 returns number of bits required to represent latencyUs
	// For latencyUs=1: bits.Len64(1)=1 -> bucket 0
	// For latencyUs=2..3: bits.Len64=2 -> bucket 1
	// For latencyUs=4..7: bits.Len64=3 -> bucket 2, etc.
	b := bits.Len64(latencyUs) - 1
	if b >= LatencyBucketCount {
		b = LatencyBucketCount - 1
	}
	return b
}

// Snapshot captures a consistent point-in-time snapshot of the telemetry counters.
func (m *MetricsSubscriber) Snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		Name:                 m.name,
		TotalReceived:        atomic.LoadUint64(&m.totalReceived),
		TotalDispatched:      atomic.LoadUint64(&m.totalDispatched),
		TotalDropped:         atomic.LoadUint64(&m.totalDropped),
		TotalFailures:        atomic.LoadUint64(&m.totalFailures),
		CircuitBreakerTrips:  atomic.LoadUint64(&m.circuitBreakerTrips),
		LastEventTimestampNs: atomic.LoadInt64(&m.lastEventTimestampNs),
	}
	for i := 0; i < LatencyBucketCount; i++ {
		s.LatencyBuckets[i] = atomic.LoadUint64(&m.latencyBuckets[i])
	}
	return s
}

// Reset clears all counters to zero.
func (m *MetricsSubscriber) Reset() {
	atomic.StoreUint64(&m.totalReceived, 0)
	atomic.StoreUint64(&m.totalDispatched, 0)
	atomic.StoreUint64(&m.totalDropped, 0)
	atomic.StoreUint64(&m.totalFailures, 0)
	atomic.StoreUint64(&m.circuitBreakerTrips, 0)
	atomic.StoreInt64(&m.lastEventTimestampNs, 0)
	for i := 0; i < LatencyBucketCount; i++ {
		atomic.StoreUint64(&m.latencyBuckets[i], 0)
	}
}

// ExportPrometheus formats the current metrics snapshot into standard Prometheus exposition format.
func (m *MetricsSubscriber) ExportPrometheus() string {
	s := m.Snapshot()
	var b strings.Builder

	fmt.Fprintf(&b, "# HELP zqk_callback_events_total Total number of callback events processed by subscriber\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_events_total counter\n")
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"received\"} %d\n", s.Name, s.TotalReceived)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"dispatched\"} %d\n", s.Name, s.TotalDispatched)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"dropped\"} %d\n", s.Name, s.TotalDropped)
	fmt.Fprintf(&b, "zqk_callback_events_total{subscriber=%q,status=\"failures\"} %d\n", s.Name, s.TotalFailures)

	fmt.Fprintf(&b, "# HELP zqk_callback_circuit_breaker_trips_total Total number of circuit breaker trips\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_circuit_breaker_trips_total counter\n")
	fmt.Fprintf(&b, "zqk_callback_circuit_breaker_trips_total{subscriber=%q} %d\n", s.Name, s.CircuitBreakerTrips)

	fmt.Fprintf(&b, "# HELP zqk_callback_dispatch_latency_microseconds_bucket Power-of-two histogram of dispatch latency in microseconds\n")
	fmt.Fprintf(&b, "# TYPE zqk_callback_dispatch_latency_microseconds_bucket histogram\n")
	var cumulative uint64
	for i := 0; i < LatencyBucketCount; i++ {
		cumulative += s.LatencyBuckets[i]
		le := 1 << i
		if i == LatencyBucketCount-1 {
			fmt.Fprintf(&b, "zqk_callback_dispatch_latency_microseconds_bucket{subscriber=%q,le=\"+Inf\"} %d\n", s.Name, cumulative)
		} else {
			fmt.Fprintf(&b, "zqk_callback_dispatch_latency_microseconds_bucket{subscriber=%q,le=\"%d\"} %d\n", s.Name, le, cumulative)
		}
	}
	fmt.Fprintf(&b, "zqk_callback_dispatch_latency_microseconds_count{subscriber=%q} %d\n", s.Name, cumulative)

	return b.String()
}
