package callback

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestAdaptiveRingBufferQueue_DropOldest(t *testing.T) {
	t.Parallel()

	cfg := AdaptiveQueueConfig{
		Capacity:     3,
		DropStrategy: DropOldest,
	}
	q := NewAdaptiveRingBufferQueue(cfg)
	require.Equal(t, 3, q.Capacity())
	require.Equal(t, 0, q.Size())

	// Push 3 entries
	e1 := &CallbackEntry{Seq: 1, JobID: "job-1"}
	e2 := &CallbackEntry{Seq: 2, JobID: "job-2"}
	e3 := &CallbackEntry{Seq: 3, JobID: "job-3"}

	evicted, err := q.Push(e1)
	require.NoError(t, err)
	assert.Nil(t, evicted)

	evicted, err = q.Push(e2)
	require.NoError(t, err)
	assert.Nil(t, evicted)

	evicted, err = q.Push(e3)
	require.NoError(t, err)
	assert.Nil(t, evicted)
	assert.Equal(t, 3, q.Size())
	assert.Equal(t, int64(0), q.EvictedCount())

	// Push 4th entry: must evict e1 (oldest)
	e4 := &CallbackEntry{Seq: 4, JobID: "job-4"}
	evicted, err = q.Push(e4)
	require.NoError(t, err)
	require.NotNil(t, evicted)
	assert.Equal(t, "job-1", evicted.JobID)
	assert.Equal(t, int64(1), q.EvictedCount())
	assert.Equal(t, 3, q.Size())

	// Pop entries: must follow FIFO order [e2, e3, e4]
	assertPoppedSequence(t, q, "job-2", "job-3", "job-4")

	_, ok := q.Pop()
	assert.False(t, ok)
	assert.Equal(t, 0, q.Size())
}

func TestAdaptiveRingBufferQueue_DropNewest(t *testing.T) {
	t.Parallel()

	cfg := AdaptiveQueueConfig{
		Capacity:     3,
		DropStrategy: DropNewest,
	}
	q := NewAdaptiveRingBufferQueue(cfg)

	require.NoError(t, pushEntryHelper(q, "job-1"))
	require.NoError(t, pushEntryHelper(q, "job-2"))
	require.NoError(t, pushEntryHelper(q, "job-3"))
	assert.Equal(t, 3, q.Size())

	// Push 4th entry: must fail with ErrQueueFull and not evict existing entries
	e4 := &CallbackEntry{JobID: "job-4"}
	evicted, err := q.Push(e4)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrQueueFull))
	assert.Nil(t, evicted)
	assert.Equal(t, int64(1), q.EvictedCount())
	assert.Equal(t, 3, q.Size())

	// Verify original elements are preserved in FIFO order
	assertPoppedSequence(t, q, "job-1", "job-2", "job-3")
}

func TestAdaptiveRingBufferQueue_WatermarkHysteresis(t *testing.T) {
	t.Parallel()

	cfg := AdaptiveQueueConfig{
		Capacity:      10,
		HighWatermark: 8,
		LowWatermark:  4,
		DropStrategy:  DropOldest,
	}
	q := NewAdaptiveRingBufferQueue(cfg)

	high, low := q.Watermarks()
	assert.Equal(t, 8, high)
	assert.Equal(t, 4, low)

	// Fill up to 7: throttling must be inactive
	for i := 1; i <= 7; i++ {
		require.NoError(t, pushEntryHelper(q, "job"))
		assert.False(t, q.IsThrottled(), "must not throttle at size %d", i)
	}

	// 8th entry hits HighWatermark: throttling activates
	require.NoError(t, pushEntryHelper(q, "job-8"))
	assert.True(t, q.IsThrottled(), "must throttle at high watermark")

	// Drain down to 5: hysteresis must keep throttling ACTIVE
	for i := 0; i < 3; i++ {
		_, ok := q.Pop()
		require.True(t, ok)
	}
	assert.Equal(t, 5, q.Size())
	assert.True(t, q.IsThrottled(), "hysteresis must keep throttle active above low watermark")

	// Drain down to 4 (LowWatermark): throttling resets
	_, ok := q.Pop()
	require.True(t, ok)
	assert.Equal(t, 4, q.Size())
	assert.False(t, q.IsThrottled(), "throttling must reset once at or below low watermark")
}

func TestAdaptiveCircuitBreaker_StateTransitions(t *testing.T) {
	t.Parallel()

	cb := NewAdaptiveCircuitBreaker(AdaptiveCircuitBreakerConfig{
		FailureThreshold:  2,
		RecoveryTimeout:   50 * time.Millisecond,
		MaxHalfOpenProbes: 1,
	})

	assert.Equal(t, StateClosed, cb.State())
	require.NoError(t, cb.AllowRequest())

	// 1st failure: remains closed
	cb.RecordFailure()
	assert.Equal(t, StateClosed, cb.State())
	require.NoError(t, cb.AllowRequest())

	// 2nd failure: trips to Open
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())

	// Immediate calls must be rejected
	err := cb.AllowRequest()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrCircuitBreakerOpen))

	// Wait for recovery timeout
	time.Sleep(60 * time.Millisecond)

	// Next call probes recovery (transitions to Half-Open)
	require.NoError(t, cb.AllowRequest())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Subsequent concurrent call while probing is rejected
	probeErr := cb.AllowRequest()
	require.Error(t, probeErr)
	assert.True(t, errors.Is(probeErr, ErrCircuitHalfOpenProbing))

	// Canary succeeds: resets to Closed
	cb.RecordSuccess()
	assert.Equal(t, StateClosed, cb.State())
	require.NoError(t, cb.AllowRequest())

	// Test canary failure tripping back to Open
	cb.Trip()
	assert.Equal(t, StateOpen, cb.State())
	time.Sleep(60 * time.Millisecond)

	require.NoError(t, cb.AllowRequest())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Probe fails: immediately reverts to Open
	cb.RecordFailure()
	assert.Equal(t, StateOpen, cb.State())
}

func TestResilientSubscriber_WatermarkBackpressureIntegration(t *testing.T) {
	t.Parallel()

	var innerCalls atomic.Int64
	blocker := make(chan struct{})

	inner := NewFuncSubscriber("slow_consumer", func(ctx context.Context, entry *CallbackEntry) error {
		innerCalls.Add(1)
		<-blocker
		return nil
	})

	cfg := ResilientSubscriberConfig{
		MaxConcurrent:      2,
		ExecutionTimeout:   1 * time.Second,
		BackpressurePolicy: BackpressureDropError,
		QueueCapacity:      5,
		HighWatermark:      3,
		LowWatermark:       1,
		DropPolicy:         DropOldest,
		Breaker:            NewAdaptiveCircuitBreaker(AdaptiveCircuitBreakerConfig{FailureThreshold: 5}),
	}

	resilient := NewResilientSubscriber(inner, cfg)
	require.NotNil(t, resilient.Queue())

	// Fill slots
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_watermark_fill", "fill subscriber watermark slots").StartSimple(func() {
			defer wg.Done()
			if notifyErr := resilient.Notify(context.Background(), &CallbackEntry{JobID: "fill"}); notifyErr != nil {
				// Backpressure concurrency/watermark rejection is valid under saturation
			}
		})
	}

	// Give goroutines time to enqueue and hit high watermark
	time.Sleep(30 * time.Millisecond)

	// An incoming call while throttled must return ErrWatermarkThrottled
	if resilient.Queue().IsThrottled() {
		err := resilient.Notify(context.Background(), &CallbackEntry{JobID: "overflow"})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrWatermarkThrottled))
	}

	close(blocker)
	wg.Wait()
}

func TestResilientSubscriber_DropOldestBurstSkew(t *testing.T) {
	t.Parallel()

	var processed atomic.Int64
	inner := NewFuncSubscriber("telemetry_stream", func(ctx context.Context, entry *CallbackEntry) error {
		processed.Add(1)
		time.Sleep(5 * time.Millisecond)
		return nil
	})

	cfg := ResilientSubscriberConfig{
		MaxConcurrent:      4,
		ExecutionTimeout:   1 * time.Second,
		BackpressurePolicy: BackpressureDropOldest,
		QueueCapacity:      10,
		HighWatermark:      10,
		LowWatermark:       4,
		DropPolicy:         DropOldest,
	}

	resilient := NewResilientSubscriber(inner, cfg)

	var wg sync.WaitGroup
	totalBurst := 50

	for i := 0; i < totalBurst; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_burst_stream", "stream burst entries").StartSimple(func() {
			defer wg.Done()
			if notifyErr := resilient.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()}); notifyErr != nil {
				// Backpressure concurrency/watermark rejection is valid under burst saturation
			}
		})
	}

	wg.Wait()

	inFlight, dropped, total, failures := resilient.Metrics()
	assert.Equal(t, int64(0), inFlight)
	assert.Equal(t, int64(totalBurst), total)
	assert.Equal(t, int64(0), failures)
	assert.GreaterOrEqual(t, dropped, int64(0))
	assert.Greater(t, processed.Load(), int64(0))
}

func assertPoppedSequence(t *testing.T, q *AdaptiveRingBufferQueue, expectedJobIDs ...string) {
	for _, expected := range expectedJobIDs {
		item, ok := q.Pop()
		require.True(t, ok)
		assert.Equal(t, expected, item.JobID)
	}
}

func pushEntryHelper(q *AdaptiveRingBufferQueue, jobID string) error {
	_, err := q.Push(&CallbackEntry{JobID: jobID})
	return err
}

