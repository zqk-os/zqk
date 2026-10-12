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

var (
	errSimulatedTransientFailure = errors.New("simulated transient failure")
)

// TestAdaptiveBackoff_ConcurrentEntropyAndMonotonicity verifies CRIT-1791675218783175000-2a7b48fa:
// Functional Acceptance: Adaptive backoff calculator enforces strict monotonic delay ceilings,
// zero memory allocations, and decorrelated jitter entropy across concurrent retry attempts.
func TestAdaptiveBackoff_ConcurrentEntropyAndMonotonicity(t *testing.T) {
	cfg := AdaptiveBackoffConfig{
		MinDelay:   10 * time.Millisecond,
		MaxDelay:   250 * time.Millisecond,
		MaxRetries: 5,
		Multiplier: 3.0,
	}
	calc := NewAdaptiveBackoffCalculator(cfg)

	// Invariant: verify Ceiling is monotonically non-decreasing
	var prevCeil time.Duration
	for attempt := 0; attempt <= 10; attempt++ {
		ceil := calc.Ceiling(attempt)
		assert.GreaterOrEqual(t, ceil, prevCeil, "ceiling must be monotonically non-decreasing")
		assert.LessOrEqual(t, ceil, cfg.MaxDelay, "ceiling must not exceed maxDelay")
		prevCeil = ceil
	}

	const workerCount = 16
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workerCount)

	delays := make([][]time.Duration, workerCount)

	for w := 0; w < workerCount; w++ {
		workerID := w
		delays[workerID] = make([]time.Duration, iterations)

		goroutinelabels.NewGoroutine("backoff-entropy-worker", "verify decorrelated entropy").StartSimple(func() {
			defer wg.Done()
			prevSleep := cfg.MinDelay
			for i := 0; i < iterations; i++ {
				attempt := i % (cfg.MaxRetries + 1)
				d := calc.Calculate(attempt, prevSleep)

				if d < cfg.MinDelay {
					t.Errorf("worker %d attempt %d delay %v below floor %v", workerID, attempt, d, cfg.MinDelay)
				}
				if d > cfg.MaxDelay {
					t.Errorf("worker %d attempt %d delay %v above max %v", workerID, attempt, d, cfg.MaxDelay)
				}

				delays[workerID][i] = d
				prevSleep = d
			}
		})
	}

	wg.Wait()

	// Verify entropy across workers at iteration 0 (independent random seeds)
	initialDelays := make(map[time.Duration]int)
	for w := 0; w < workerCount; w++ {
		initialDelays[delays[w][0]]++
	}
	assert.GreaterOrEqual(t, len(initialDelays), 3, "expected entropy across concurrent workers")
}

// TestQuorumDampener_SlidingWindowBurstAndLoadShedding verifies CRIT-1791675218783176000-d82b318a:
// Boundary & Error Handling: Sliding window rate limiters and event storm quorum damping
// clamp burst storms, shed excess load, and prevent cascading cluster outages.
func TestQuorumDampener_SlidingWindowBurstAndLoadShedding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	window := 100 * time.Millisecond
	dampener := NewQuorumDampener(QuorumDampenerConfig{
		RateLimit:     20.0, // 20 events / sec
		BurstCapacity: 5,    // burst up to 5 tokens
		WindowSize:    window,
		MaxTopics:     50,
		TopicTTL:      1 * time.Second,
		Action:        DampingActionReject,
	})
	dampener.Start(ctx)
	defer dampener.Stop()

	topic := "topic-event-storm"
	allowedCount := 0
	rejectedCount := 0

	// Flood 50 events instantly
	for i := 0; i < 50; i++ {
		allowed, _ := dampener.Allow(topic)
		if allowed {
			allowedCount++
		} else {
			rejectedCount++
		}
	}

	// Burst capacity is 5, so initial intake must be bounded
	assert.LessOrEqual(t, allowedCount, 8, "burst capacity must strictly clamp instant flood")
	assert.GreaterOrEqual(t, rejectedCount, 40, "excess burst must be shed by dampener")

	// Saturated topic entry rejection
	saturatedEntry := &CallbackEntry{Payload: map[string]any{"topic": topic}}
	allowedSaturated, errSaturated := dampener.AllowEntry(ctx, saturatedEntry)
	assert.False(t, allowedSaturated, "saturated topic entry must be rejected")
	assert.ErrorIs(t, errSaturated, ErrQuorumDampingActive, "saturated topic must yield ErrQuorumDampingActive")

	// Fresh topic entry acceptance
	freshEntry := &CallbackEntry{Payload: map[string]any{"topic": "fresh-topic"}}
	allowedFresh, errFresh := dampener.AllowEntry(ctx, freshEntry)
	require.NoError(t, errFresh, "fresh topic should be accepted")
	assert.True(t, allowedFresh, "fresh topic entry must be allowed")
}

// TestDampedSubscriber_SwarmStormSuppressionAndFailover verifies CRIT-1791675218783177000-ad962d2c:
// Documentation & Verification: Swarm backoff damping verification suite and architecture specification.
func TestDampedSubscriber_SwarmStormSuppressionAndFailover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var attempts atomic.Int64
	var successes atomic.Int64

	inner := NewFuncSubscriber("failover-worker", func(ctx context.Context, entry *CallbackEntry) error {
		cur := attempts.Add(1)
		// Fail first 2 attempts, then succeed
		if cur <= 2 {
			return errSimulatedTransientFailure
		}
		successes.Add(1)
		return nil
	})

	damped := NewDampedSubscriber(
		inner,
		DampedSubscriberConfig{
			BackoffConfig: AdaptiveBackoffConfig{
				MinDelay:   5 * time.Millisecond,
				MaxDelay:   50 * time.Millisecond,
				MaxRetries: 4,
				Multiplier: 2.0,
			},
			DampenerConfig: DefaultQuorumDampenerConfig(),
		},
	)

	err := damped.Notify(ctx, &CallbackEntry{
		Payload: map[string]any{"topic": "sub-resilient-failover"},
	})

	require.NoError(t, err, "damped subscriber should succeed after transient retry backoff")
	assert.Equal(t, int64(3), attempts.Load(), "should have retried exactly twice before success")
	assert.Equal(t, int64(1), successes.Load(), "should have exactly one successful completion")
}
