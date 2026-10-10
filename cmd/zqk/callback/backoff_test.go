package callback

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// CRIT-1791675181490037000-624f16d8: Verify Adaptive Backoff & Decorrelated Exponential Jitter
func TestAdaptiveBackoffCalculator_FullJitterBounds(t *testing.T) {
	minDelay := 20 * time.Millisecond
	maxDelay := 200 * time.Millisecond
	calc := NewAdaptiveBackoffCalculator(AdaptiveBackoffConfig{
		MinDelay:   minDelay,
		MaxDelay:   maxDelay,
		MaxRetries: 5,
		Multiplier: 3.0,
	})

	assert.Equal(t, minDelay, calc.MinDelay())
	assert.Equal(t, maxDelay, calc.MaxDelay())
	assert.Equal(t, 5, calc.MaxRetries())

	// Test 1000 iterations across varying sleep seeds
	prevSleep := minDelay
	for i := 0; i < 1000; i++ {
		attempt := i % 10
		delay := calc.Calculate(attempt, prevSleep)

		assert.GreaterOrEqual(t, delay, minDelay, "calculated delay must be >= minDelay")
		assert.LessOrEqual(t, delay, maxDelay, "calculated delay must be <= maxDelay")
		prevSleep = delay
	}
}

func TestAdaptiveBackoffCalculator_MonotonicCeilingEnforcement(t *testing.T) {
	minDelay := 10 * time.Millisecond
	maxDelay := 320 * time.Millisecond
	calc := NewAdaptiveBackoffCalculator(AdaptiveBackoffConfig{
		MinDelay:   minDelay,
		MaxDelay:   maxDelay,
		MaxRetries: 10,
	})

	var prevCeiling time.Duration
	for attempt := 0; attempt <= 15; attempt++ {
		ceiling := calc.Ceiling(attempt)
		assert.GreaterOrEqual(t, ceiling, prevCeiling, "ceiling must be monotonically non-decreasing")
		assert.LessOrEqual(t, ceiling, maxDelay, "ceiling must not exceed maxDelay")
		prevCeiling = ceiling
	}

	// Verify upper ceiling caps at maxDelay
	assert.Equal(t, maxDelay, calc.Ceiling(35))
	assert.Equal(t, maxDelay, calc.Ceiling(100))
}

func TestAdaptiveBackoffCalculator_ZeroAllocation(t *testing.T) {
	calc := NewAdaptiveBackoffCalculator(DefaultAdaptiveBackoffConfig())
	allocs := testing.AllocsPerRun(1000, func() {
		_ = calc.Calculate(1, 100*time.Millisecond)
	})
	assert.Equal(t, float64(0), allocs, "Calculate must not allocate heap memory")
}

// CRIT-1791675181490038000-f3bddf4b: Verify Boundary & Error Handling
func TestAdaptiveBackoffCalculator_NegativeAndBoundaryConfig(t *testing.T) {
	// Zero and negative config values should fall back to safe defaults
	calc := NewAdaptiveBackoffCalculator(AdaptiveBackoffConfig{
		MinDelay:   -10 * time.Millisecond,
		MaxDelay:   -50 * time.Millisecond,
		MaxRetries: -1,
		Multiplier: -2.0,
	})

	assert.Equal(t, DefaultMinBackoffDelay, calc.MinDelay())
	assert.GreaterOrEqual(t, calc.MaxDelay(), calc.MinDelay())
	assert.Equal(t, 0, calc.MaxRetries())
	assert.Equal(t, DefaultBackoffMultiplier, calc.Config().Multiplier)

	calc2 := NewAdaptiveBackoffCalculator(AdaptiveBackoffConfig{
		MinDelay:   200 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
		MaxRetries: 3,
	})
	assert.Equal(t, 200*time.Millisecond, calc2.MinDelay())
	assert.Equal(t, 200*time.Millisecond, calc2.MaxDelay())

	// CanRetry boundary
	assert.False(t, calc.CanRetry(0))  // MaxRetries=0 means 0 retries allowed
	assert.True(t, calc2.CanRetry(0))  // MaxRetries=3 allows retry
	assert.False(t, calc2.CanRetry(3)) // Attempt 3 reaches limit
}

func TestAdaptiveBackoffCalculator_OverflowPrevention(t *testing.T) {
	calc := NewAdaptiveBackoffCalculator(AdaptiveBackoffConfig{
		MinDelay:   100 * time.Millisecond,
		MaxDelay:   10 * time.Second,
		Multiplier: 3.0,
	})

	// Pass extreme durations that would overflow int64 when multiplied by 3
	extremeValues := []time.Duration{
		math.MaxInt64,
		math.MaxInt64 - 100,
		time.Duration(math.MaxInt64 / 2),
	}

	for _, extreme := range extremeValues {
		delay := calc.Calculate(1, extreme)
		assert.GreaterOrEqual(t, delay, calc.MinDelay())
		assert.LessOrEqual(t, delay, calc.MaxDelay())
	}
}

// CRIT-1791675181490038000-f3bddf4b: Sliding Window Rate Limiting & Quorum Event Damping
func TestQuorumDampener_RateLimitingAndStormSuppression(t *testing.T) {
	fakeNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clockMu := sync.Mutex{}
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return fakeNow
	}

	dampener := NewQuorumDampener(QuorumDampenerConfig{
		RateLimit:     10.0,
		BurstCapacity: 5,
		WindowSize:    1 * time.Second,
		Action:        DampingActionReject,
		Clock:         clock,
	})

	topic := "build_event"

	// Initial burst of 5 should succeed
	for i := 0; i < 5; i++ {
		err := dampener.Check(topic)
		require.NoError(t, err, "burst event %d should be allowed", i)
	}

	// 6th event in the same instant must trigger ErrQuorumDampingActive
	err := dampener.Check(topic)
	require.ErrorIs(t, err, ErrQuorumDampingActive, "storm event beyond burst capacity must be damped")

	// Advance clock by 500ms -> should refill 5 tokens (10 tokens/sec * 0.5s = 5)
	clockMu.Lock()
	fakeNow = fakeNow.Add(500 * time.Millisecond)
	clockMu.Unlock()

	for i := 0; i < 5; i++ {
		err := dampener.Check(topic)
		require.NoError(t, err, "refilled event %d should be allowed", i)
	}

	// Saturated again
	err = dampener.Check(topic)
	require.ErrorIs(t, err, ErrQuorumDampingActive)

	// Verify metrics
	allowed, damped, dropped := dampener.Metrics()
	assert.Equal(t, int64(10), allowed)
	assert.Equal(t, int64(2), damped)
	assert.Equal(t, int64(0), dropped)
}

func TestQuorumDampener_CollapseAndDropAction(t *testing.T) {
	fakeNow := time.Now()
	clock := func() time.Time { return fakeNow }

	// 1. Collapse Action
	collapseDampener := NewQuorumDampener(QuorumDampenerConfig{
		RateLimit:     5.0,
		BurstCapacity: 2,
		WindowSize:    1 * time.Second,
		Action:        DampingActionCollapse,
		Clock:         clock,
	})

	topic := "git_push"
	// Consume burst
	ok, err := collapseDampener.Allow(topic)
	assert.True(t, ok)
	assert.NoError(t, err)
	ok, err = collapseDampener.Allow(topic)
	assert.True(t, ok)
	assert.NoError(t, err)

	// 3rd event should be collapsed (no error, ok = false)
	ok, err = collapseDampener.Allow(topic)
	assert.False(t, ok)
	assert.NoError(t, err)

	_, collapsed, total, found := collapseDampener.TopicStats(topic)
	assert.True(t, found)
	assert.Equal(t, int64(1), collapsed)
	assert.Equal(t, int64(3), total)

	// 2. Drop Action
	dropDampener := NewQuorumDampener(QuorumDampenerConfig{
		RateLimit:     5.0,
		BurstCapacity: 1,
		WindowSize:    1 * time.Second,
		Action:        DampingActionDrop,
		Clock:         clock,
	})

	ok, err = dropDampener.Allow(topic)
	assert.True(t, ok)
	assert.NoError(t, err)

	// Damped event dropped silently
	ok, err = dropDampener.Allow(topic)
	assert.False(t, ok)
	assert.NoError(t, err)

	_, _, dropped := dropDampener.Metrics()
	assert.Equal(t, int64(1), dropped)
}

func TestQuorumDampener_BoundedMemoryAndEviction(t *testing.T) {
	fakeNow := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	clockMu := sync.Mutex{}
	clock := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return fakeNow
	}

	maxTopics := 5
	dampener := NewQuorumDampener(QuorumDampenerConfig{
		MaxTopics: maxTopics,
		TopicTTL:  1 * time.Minute,
		Clock:     clock,
	})

	// Add 5 distinct topics
	topics := []string{"t1", "t2", "t3", "t4", "t5"}
	for _, top := range topics {
		_, _ = dampener.Allow(top)
	}
	assert.Equal(t, 5, dampener.ActiveTopicCount())

	// Advance time past TTL
	clockMu.Lock()
	fakeNow = fakeNow.Add(2 * time.Minute)
	clockMu.Unlock()

	// Ingesting a new topic should trigger TTL eviction
	_, _ = dampener.Allow("t6")
	assert.LessOrEqual(t, dampener.ActiveTopicCount(), maxTopics)

	// Rapidly ingest 50 distinct topics without time advancing: capacity must stay bounded
	for i := 0; i < 50; i++ {
		_, _ = dampener.Allow(string(rune('A' + i)))
		assert.LessOrEqual(t, dampener.ActiveTopicCount(), maxTopics)
	}
	assert.Equal(t, maxTopics, dampener.ActiveTopicCount())
}

func TestQuorumDampener_AllowEntryAndTopicExtractor(t *testing.T) {
	dampener := NewQuorumDampener(DefaultQuorumDampenerConfig())

	ctx := context.Background()

	// 1. Entry with topic payload
	entry1 := &CallbackEntry{
		Payload: map[string]any{"topic": "agent_heartbeat"},
	}
	ok, err := dampener.AllowEntry(ctx, entry1)
	assert.True(t, ok)
	assert.NoError(t, err)

	// 2. Entry with JobID fallback
	entry2 := &CallbackEntry{
		JobID: "job-1234",
	}
	ok, err = dampener.AllowEntry(ctx, entry2)
	assert.True(t, ok)
	assert.NoError(t, err)

	// 3. Entry with canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, err = dampener.AllowEntry(canceledCtx, entry1)
	assert.False(t, ok)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestQuorumDampener_LifecycleStartStop(t *testing.T) {
	dampener := NewQuorumDampener(QuorumDampenerConfig{
		WindowSize: 10 * time.Millisecond,
		TopicTTL:   5 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dampener.Start(ctx)
	// Idempotent double start
	dampener.Start(ctx)

	_, _ = dampener.Allow("temp_topic")
	assert.Equal(t, 1, dampener.ActiveTopicCount())

	time.Sleep(30 * time.Millisecond)

	dampener.Stop()
	// Idempotent double stop
	dampener.Stop()
}

func TestDampedSubscriber_AutomaticRetriesAndBackoff(t *testing.T) {
	attempts := atomic.Int32{}
	inner := NewFuncSubscriber("flaky_worker", func(ctx context.Context, entry *CallbackEntry) error {
		if attempts.Add(1) < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	dampedSub := NewDampedSubscriber(inner, DampedSubscriberConfig{
		BackoffConfig: AdaptiveBackoffConfig{
			MinDelay:   5 * time.Millisecond,
			MaxDelay:   20 * time.Millisecond,
			MaxRetries: 4,
		},
		DampenerConfig: QuorumDampenerConfig{
			RateLimit:     100,
			BurstCapacity: 10,
		},
	})

	assert.Equal(t, "flaky_worker", dampedSub.Name())
	assert.Equal(t, inner, dampedSub.Inner())

	entry := &CallbackEntry{JobID: "test-retry"}
	err := dampedSub.Notify(context.Background(), entry)
	require.NoError(t, err, "subscriber should succeed after retries")

	assert.Equal(t, int32(3), attempts.Load(), "should have executed 3 times (1 initial + 2 retries)")
	processed, retries, damped, failures, successes := dampedSub.Metrics()
	assert.Equal(t, int64(1), processed)
	assert.Equal(t, int64(2), retries)
	assert.Equal(t, int64(0), damped)
	assert.Equal(t, int64(0), failures)
	assert.Equal(t, int64(1), successes)
}

func TestDampedSubscriber_MaxRetriesExhaustion(t *testing.T) {
	attempts := atomic.Int32{}
	expectedErr := errors.New("permanent failure")
	inner := NewFuncSubscriber("fatal_worker", func(ctx context.Context, entry *CallbackEntry) error {
		attempts.Add(1)
		return expectedErr
	})

	maxRetries := 3
	dampedSub := NewDampedSubscriber(inner, DampedSubscriberConfig{
		BackoffConfig: AdaptiveBackoffConfig{
			MinDelay:   2 * time.Millisecond,
			MaxDelay:   10 * time.Millisecond,
			MaxRetries: maxRetries,
		},
	})

	err := dampedSub.Notify(context.Background(), &CallbackEntry{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded maximum retries (3)")
	assert.Equal(t, int32(maxRetries+1), attempts.Load(), "initial attempt + maxRetries")

	_, retries, _, failures, successes := dampedSub.Metrics()
	assert.Equal(t, int64(maxRetries), retries)
	assert.Equal(t, int64(1), failures)
	assert.Equal(t, int64(0), successes)
}

func TestDampedSubscriber_QuorumDampingStormRejection(t *testing.T) {
	innerExecuted := atomic.Int64{}
	inner := NewFuncSubscriber("fast_worker", func(ctx context.Context, entry *CallbackEntry) error {
		innerExecuted.Add(1)
		return nil
	})

	dampener := NewQuorumDampener(QuorumDampenerConfig{
		RateLimit:     5.0,
		BurstCapacity: 2,
		WindowSize:    1 * time.Second,
		Action:        DampingActionReject,
	})

	dampedSub := NewDampedSubscriberWithComponents(inner, dampener, nil)

	entry := &CallbackEntry{Payload: map[string]any{"topic": "storm_topic"}}

	// First 2 succeed
	require.NoError(t, dampedSub.Notify(context.Background(), entry))
	require.NoError(t, dampedSub.Notify(context.Background(), entry))

	// 3rd must fail fast with ErrQuorumDampingActive without invoking inner
	err := dampedSub.Notify(context.Background(), entry)
	require.ErrorIs(t, err, ErrQuorumDampingActive)

	assert.Equal(t, int64(2), innerExecuted.Load(), "inner worker must not be saturated during storm")
	_, _, damped, _, _ := dampedSub.Metrics()
	assert.Equal(t, int64(1), damped)
}

func TestDampedSubscriber_ContextCancellation(t *testing.T) {
	inner := NewFuncSubscriber("slow_worker", func(ctx context.Context, entry *CallbackEntry) error {
		return errors.New("fail to force retry")
	})

	dampedSub := NewDampedSubscriber(inner, DampedSubscriberConfig{
		BackoffConfig: AdaptiveBackoffConfig{
			MinDelay:   500 * time.Millisecond,
			MaxDelay:   2 * time.Second,
			MaxRetries: 5,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := dampedSub.Notify(ctx, &CallbackEntry{})
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 200*time.Millisecond, "must abort quickly upon context expiration")
}

func TestDampedSubscriber_NonRetryableError(t *testing.T) {
	attempts := atomic.Int32{}
	inner := NewFuncSubscriber("selective_worker", func(ctx context.Context, entry *CallbackEntry) error {
		attempts.Add(1)
		return errors.New("non-retryable bad request")
	})

	dampedSub := NewDampedSubscriber(inner, DampedSubscriberConfig{
		BackoffConfig: AdaptiveBackoffConfig{
			MinDelay:   2 * time.Millisecond,
			MaxRetries: 5,
		},
		Retryable: func(err error) bool {
			return false // none retryable
		},
	})

	err := dampedSub.Notify(context.Background(), &CallbackEntry{})
	require.Error(t, err)
	assert.Equal(t, int32(1), attempts.Load(), "should not retry when error is not retryable")
}

// CRIT-1791675181490038000-f3bddf4b: High-Concurrency Throughput with Race Detector
func TestDampedSubscriber_HighConcurrencyRace(t *testing.T) {
	counter := atomic.Int64{}
	inner := NewFuncSubscriber("concurrent_sink", func(ctx context.Context, entry *CallbackEntry) error {
		counter.Add(1)
		return nil
	})

	dampedSub := NewDampedSubscriber(inner, DampedSubscriberConfig{
		BackoffConfig: AdaptiveBackoffConfig{
			MinDelay:   1 * time.Millisecond,
			MaxDelay:   5 * time.Millisecond,
			MaxRetries: 2,
		},
		DampenerConfig: QuorumDampenerConfig{
			RateLimit:     10000,
			BurstCapacity: 5000,
			MaxTopics:     20,
		},
	})

	const numGoroutines = 30
	const eventsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		gid := g
		goroutinelabels.NewGoroutine("test_damped_sub_concurrent", "concurrent notify test").StartSimple(func() {
			defer wg.Done()
			for e := 0; e < eventsPerGoroutine; e++ {
				topic := string(rune('A' + (e % 10)))
				entry := &CallbackEntry{
					Payload: map[string]any{"topic": topic},
				}
				_ = dampedSub.Notify(context.Background(), entry)
			}
			_ = gid
		})
	}

	wg.Wait()
	assert.Equal(t, int64(numGoroutines*eventsPerGoroutine), counter.Load())
	processed, _, _, _, successes := dampedSub.Metrics()
	assert.Equal(t, int64(numGoroutines*eventsPerGoroutine), processed)
	assert.Equal(t, int64(numGoroutines*eventsPerGoroutine), successes)
}
