package callback

import (
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// DropStrategy specifies the queue shedding policy when capacity is saturated.
type DropStrategy int

const (
	// DropOldest evicts the oldest queued entry to accept a new entry (default for streaming/telemetry).
	DropOldest DropStrategy = iota
	// DropNewest rejects incoming entries when capacity is full, returning ErrQueueFull.
	DropNewest
)

// String returns human-readable name of the drop strategy.
func (d DropStrategy) String() string {
	switch d {
	case DropOldest:
		return "drop_oldest"
	case DropNewest:
		return "drop_newest"
	default:
		return "unknown"
	}
}

// AdaptiveRingBufferQueue is a concurrent, bounded circular buffer for subscriber backpressure.
// It supports deterministic drop policies (DropOldest vs DropNewest) and hysteresis watermark throttling.
type AdaptiveRingBufferQueue struct {
	mu           sync.Mutex
	items        []*CallbackEntry
	head         int
	tail         int
	size         int
	capacity     int
	dropStrategy DropStrategy
	evictedCount atomic.Int64

	// Watermark throttling
	highWatermark int
	lowWatermark  int
	throttled     atomic.Bool
}

// AdaptiveQueueConfig configures capacity, drop behavior, and watermark thresholds.
type AdaptiveQueueConfig struct {
	Capacity      int
	DropStrategy  DropStrategy
	HighWatermark int // Threshold at which throttling activates (defaults to Capacity)
	LowWatermark  int // Threshold at which throttling resets (defaults to HighWatermark / 2)
}

// NewAdaptiveRingBufferQueue creates a bounded ring buffer with configurable drop strategy and watermarks.
func NewAdaptiveRingBufferQueue(cfg AdaptiveQueueConfig) *AdaptiveRingBufferQueue {
	capacity := cfg.Capacity
	if capacity <= 0 {
		capacity = 64
	}

	highWM := cfg.HighWatermark
	if highWM <= 0 || highWM > capacity {
		highWM = capacity
	}

	lowWM := cfg.LowWatermark
	if lowWM <= 0 || lowWM >= highWM {
		lowWM = highWM / 2
		if lowWM < 1 {
			lowWM = 1
		}
	}

	return &AdaptiveRingBufferQueue{
		items:         make([]*CallbackEntry, capacity),
		capacity:      capacity,
		dropStrategy:  cfg.DropStrategy,
		highWatermark: highWM,
		lowWatermark:  lowWM,
	}
}

// Push adds an entry to the queue. When the queue is saturated, it executes
// the configured DropStrategy (either evicting the oldest entry or returning ErrQueueFull).
func (q *AdaptiveRingBufferQueue) Push(entry *CallbackEntry) (evicted *CallbackEntry, err error) {
	if entry == nil {
		return nil, nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == q.capacity {
		if q.dropStrategy == DropNewest {
			q.evictedCount.Add(1)
			return nil, ErrQueueFull
		}

		// DropOldest: evict head item to make room
		evicted = q.items[q.head]
		q.items[q.head] = nil // Avoid memory retention
		q.head = (q.head + 1) % q.capacity
		q.evictedCount.Add(1)

		q.items[q.tail] = entry
		q.tail = (q.tail + 1) % q.capacity
		q.updateWatermarksLocked()
		return evicted, nil
	}

	q.items[q.tail] = entry
	q.tail = (q.tail + 1) % q.capacity
	q.size++

	q.updateWatermarksLocked()
	return nil, nil
}

func (q *AdaptiveRingBufferQueue) headEntryLocked() (*CallbackEntry, bool) {
	if q.size == 0 {
		return nil, false
	}
	return q.items[q.head], true
}

// Pop extracts the oldest entry in FIFO order. Returns false if the queue is empty.
func (q *AdaptiveRingBufferQueue) Pop() (*CallbackEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	entry, ok := q.headEntryLocked()
	if !ok {
		return nil, false
	}

	q.items[q.head] = nil
	q.head = (q.head + 1) % q.capacity
	q.size--

	q.updateWatermarksLocked()
	return entry, true
}

// Peek returns the oldest entry without removing it. Returns false if empty.
func (q *AdaptiveRingBufferQueue) Peek() (*CallbackEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.headEntryLocked()
}

// Drain extracts all active entries in FIFO order, resetting the queue.
func (q *AdaptiveRingBufferQueue) Drain() []*CallbackEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == 0 {
		return nil
	}

	drained := make([]*CallbackEntry, 0, q.size)
	for q.size > 0 {
		entry := q.items[q.head]
		q.items[q.head] = nil
		q.head = (q.head + 1) % q.capacity
		q.size--
		if entry != nil {
			drained = append(drained, entry)
		}
	}
	q.throttled.Store(false)
	return drained
}

// Size returns the active count in the queue.
func (q *AdaptiveRingBufferQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

// Capacity returns the total allocated capacity.
func (q *AdaptiveRingBufferQueue) Capacity() int {
	return q.capacity
}

// EvictedCount returns total items dropped due to saturation.
func (q *AdaptiveRingBufferQueue) EvictedCount() int64 {
	return q.evictedCount.Load()
}

// IsThrottled reports whether queue depth currently exceeds high watermark.
func (q *AdaptiveRingBufferQueue) IsThrottled() bool {
	return q.throttled.Load()
}

// Watermarks returns the high and low watermark settings.
func (q *AdaptiveRingBufferQueue) Watermarks() (high, low int) {
	return q.highWatermark, q.lowWatermark
}

func (q *AdaptiveRingBufferQueue) updateWatermarksLocked() {
	if q.size >= q.highWatermark {
		q.throttled.Store(true)
	} else if q.size <= q.lowWatermark {
		q.throttled.Store(false)
	}
}

// ErrWatermarkThrottled is returned when subscriber queue depth exceeds high watermark limit.
var ErrWatermarkThrottled = errfmt.Errorf("backpressure: subscriber high watermark exceeded")
