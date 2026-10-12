package callback

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type tokenBucketHarness struct {
	t       *testing.T
	limiter *TokenBucketLimiter
	drr     *DeficitRoundRobinQueue
	manager *FlowControlManager
}

func newTokenBucketHarness(t *testing.T) *tokenBucketHarness {
	t.Helper()
	return &tokenBucketHarness{t: t}
}

func (h *tokenBucketHarness) createLimiter(cap int64, rate float64, init int64) *TokenBucketLimiter {
	h.t.Helper()
	return h.createLimiterWithClock(cap, rate, init, nil)
}

func (h *tokenBucketHarness) expectDequeue(drr *DeficitRoundRobinQueue, expectedSub, expectedJob string) {
	h.t.Helper()
	item, err := drr.Dequeue()
	assert.NoError(h.t, err)
	require.NotNil(h.t, item)
	assert.Equal(h.t, expectedSub, item.SubscriberID)
	assert.Equal(h.t, expectedJob, item.Entry.JobID)
}

func (h *tokenBucketHarness) createLimiterWithClock(cap int64, rate float64, init int64, clock func() time.Time) *TokenBucketLimiter {
	h.t.Helper()
	cfg := TokenBucketConfig{
		Capacity:   cap,
		RefillRate: rate,
		Clock:      clock,
	}
	if init == 0 {
		cfg.StartEmpty = true
	}
	if init > 0 {
		cfg.InitialTokens = init
	}
	lim, err := NewTokenBucketLimiter(cfg)
	require.NoError(h.t, err)
	h.limiter = lim
	h.t.Cleanup(func() {
		errClose := lim.Close()
		assert.NoError(h.t, errClose)
	})
	return lim
}

func (h *tokenBucketHarness) createDRRQueue(quantum, maxDeficit int64, depth int) *DeficitRoundRobinQueue {
	h.t.Helper()
	drr := NewDeficitRoundRobinQueue(DeficitRoundRobinConfig{
		DefaultQuantum:    quantum,
		DefaultMaxDeficit: maxDeficit,
		DefaultMaxDepth:   depth,
	})
	require.NotNil(h.t, drr)
	h.drr = drr
	h.t.Cleanup(func() {
		errClose := drr.Close()
		assert.NoError(h.t, errClose)
	})
	return drr
}

func (h *tokenBucketHarness) createFlowControlManager(cap int64, rate float64, quantum, maxDeficit int64) *FlowControlManager {
	h.t.Helper()
	mgr, err := NewFlowControlManager(FlowControlConfig{
		Limiter: TokenBucketConfig{
			Capacity:      cap,
			RefillRate:    rate,
			InitialTokens: cap,
		},
		Queue: DeficitRoundRobinConfig{
			DefaultQuantum:    quantum,
			DefaultMaxDeficit: maxDeficit,
			DefaultMaxDepth:   500,
		},
	})
	require.NoError(h.t, err)
	h.manager = mgr
	h.t.Cleanup(func() {
		errClose := mgr.Close()
		assert.NoError(h.t, errClose)
	})
	return mgr
}

func (h *tokenBucketHarness) newEntry(jobID string, priority int) *CallbackEntry {
	h.t.Helper()
	return &CallbackEntry{
		JobID:     jobID,
		Priority:  priority,
		Timestamp: time.Now(),
		Payload:   map[string]any{"job": jobID},
	}
}

// -----------------------------------------------------------------------------
// 1. TokenBucketLimiter Unit Tests
// -----------------------------------------------------------------------------

func TestTokenBucket_ConfigurationValidation(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)

	_, err1 := NewTokenBucketLimiter(TokenBucketConfig{Capacity: 0, RefillRate: 10})
	assert.ErrorIs(t, err1, ErrInvalidConfiguration)

	_, err2 := NewTokenBucketLimiter(TokenBucketConfig{Capacity: 100, RefillRate: -1})
	assert.ErrorIs(t, err2, ErrInvalidConfiguration)

	lim := h.createLimiter(100, 50, 100)
	assert.Equal(t, int64(100), lim.Capacity())
	assert.Equal(t, 50.0, lim.RefillRate())
	assert.Equal(t, int64(100), lim.AvailableTokens())
}

func TestTokenBucket_BurstCapacityAndReplenishment(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)

	now := time.Now()
	var clockMu sync.Mutex
	currentClock := now

	clockFn := func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return currentClock
	}

	advanceClock := func(d time.Duration) {
		clockMu.Lock()
		defer clockMu.Unlock()
		currentClock = currentClock.Add(d)
	}

	lim := h.createLimiterWithClock(100, 10, 50, clockFn)
	assert.Equal(t, int64(50), lim.AvailableTokens())

	// Consume 50 tokens
	ok := lim.TryAcquire(50)
	assert.True(t, ok)
	assert.Equal(t, int64(0), lim.AvailableTokens())

	// Depleted
	ok2 := lim.TryAcquire(1)
	assert.False(t, ok2)

	// Advance clock by 2 seconds at 10 tokens/sec -> +20 tokens
	advanceClock(2 * time.Second)
	assert.Equal(t, int64(20), lim.AvailableTokens())

	// Advance clock by 20 seconds -> capped at capacity (100)
	advanceClock(20 * time.Second)
	assert.Equal(t, int64(100), lim.AvailableTokens())

	// Cannot exceed capacity
	advanceClock(10 * time.Second)
	assert.Equal(t, int64(100), lim.AvailableTokens())
}

func TestTokenBucket_TryAcquire_EdgeCases(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	lim := h.createLimiter(50, 10, 50)

	assert.False(t, lim.TryAcquire(0))
	assert.False(t, lim.TryAcquire(-5))
	assert.False(t, lim.TryAcquire(51))
	assert.True(t, lim.TryAcquire(25))
	assert.Equal(t, int64(25), lim.AvailableTokens())

	acq, rej := lim.Stats()
	assert.Equal(t, int64(25), acq)
	assert.Equal(t, int64(0), rej)

	assert.False(t, lim.TryAcquire(30))
	acq2, rej2 := lim.Stats()
	assert.Equal(t, int64(25), acq2)
	assert.Equal(t, int64(1), rej2)
}

func TestTokenBucket_BlockingAcquire_SuccessAndTimeout(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	lim := h.createLimiter(10, 100, 0) // Starts empty, refills at 100/sec

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := lim.Acquire(ctx, 5)
	assert.NoError(t, err)

	// Test acquire error on invalid tokens
	assert.ErrorIs(t, lim.Acquire(ctx, 0), ErrInvalidTokenRequest)
	assert.ErrorIs(t, lim.Acquire(ctx, 15), ErrTokenRequestExceedsCap)

	// Test timeout with 0 refill rate
	limStall := h.createLimiter(10, 0, 0)
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancelTimeout()

	errTimeout := limStall.Acquire(ctxTimeout, 5)
	assert.ErrorIs(t, errTimeout, context.DeadlineExceeded)
}

func TestTokenBucket_CloseUnblocksWaiters(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	lim := h.createLimiter(10, 0, 0)

	waiterDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_tb_waiter", "wait on empty limiter").StartSimple(func() {
		err := lim.Acquire(context.Background(), 5)
		waiterDone <- err
	})

	time.Sleep(20 * time.Millisecond)
	errClose := lim.Close()
	assert.NoError(t, errClose)

	select {
	case err := <-waiterDone:
		assert.ErrorIs(t, err, ErrLimiterClosed)
	case <-time.After(1 * time.Second):
		t.Fatal("waiter did not unblock on Close()")
	}
}

// -----------------------------------------------------------------------------
// 2. DeficitRoundRobinQueue Unit & Starvation Tests
// -----------------------------------------------------------------------------

func TestDeficitRoundRobin_FairQueueingAndStarvationPrevention(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	drr := h.createDRRQueue(100, 500, 100)

	// Register sub-A with quantum 100, sub-B with quantum 100
	errRegA := drr.RegisterSubscriber("sub-A", 100, 500, 100)
	assert.NoError(t, errRegA)
	errRegB := drr.RegisterSubscriber("sub-B", 100, 500, 100)
	assert.NoError(t, errRegB)

	// Sub-A is a "heavy" subscriber sending items of cost 100
	// Sub-B is a "light" subscriber sending items of cost 50
	for i := 0; i < 4; i++ {
		eA := h.newEntry(fmt.Sprintf("job-A-%d", i), 1)
		errEA := drr.Enqueue("sub-A", eA, 100)
		assert.NoError(t, errEA)

		eB := h.newEntry(fmt.Sprintf("job-B-%d", i), 1)
		errEB := drr.Enqueue("sub-B", eB, 50)
		assert.NoError(t, errEB)
	}

	assert.Equal(t, 4, drr.QueueDepth("sub-A"))
	assert.Equal(t, 4, drr.QueueDepth("sub-B"))

	// Dequeue sequence:
	// Round 1:
	// sub-A gets quantum 100 -> deficit=100. Item cost 100 <= 100 -> pops item A-0, deficit=0.
	// sub-B gets quantum 100 -> deficit=100. Item cost 50 <= 100 -> pops item B-0, deficit=50.
	//   Item cost B-1 (50) <= deficit(50) -> pops item B-1, deficit=0.
	// Both subscribers make verified forward progress without starvation.
	h.expectDequeue(drr, "sub-A", "job-A-0")
	h.expectDequeue(drr, "sub-B", "job-B-0")
	h.expectDequeue(drr, "sub-B", "job-B-1")
	h.expectDequeue(drr, "sub-A", "job-A-1")
}

func TestDeficitRoundRobin_DeficitAccumulation(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	// Quantum 50, cost 100. First round accumulates 50, next round accumulates 50 -> 100 >= 100 -> pops
	drr := h.createDRRQueue(50, 200, 10)

	entry := h.newEntry("job-big", 1)
	errEnq := drr.Enqueue("sub-big", entry, 100)
	assert.NoError(t, errEnq)

	// Step 1: deficit becomes 50, insufficient for 100 cost, accumulates and round passes
	// When another subscriber exists, it gets scheduled
	errEnqSmall := drr.Enqueue("sub-small", h.newEntry("job-small", 1), 30)
	assert.NoError(t, errEnqSmall)

	// sub-big deficit=50 (<100) -> accumulates, passes to sub-small deficit=50 (>=30) -> pops sub-small
	h.expectDequeue(drr, "sub-small", "job-small")

	// Next round: sub-big deficit was 50 + quantum 50 = 100 >= 100 -> pops sub-big
	h.expectDequeue(drr, "sub-big", "job-big")
}

func TestDeficitRoundRobin_QueueFullAndClosed(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	drr := h.createDRRQueue(100, 500, 2) // Max depth 2

	assert.NoError(t, drr.Enqueue("sub-1", h.newEntry("j1", 1), 10))
	assert.NoError(t, drr.Enqueue("sub-1", h.newEntry("j2", 1), 10))
	errFull := drr.Enqueue("sub-1", h.newEntry("j3", 1), 10)
	assert.ErrorIs(t, errFull, ErrQueueFull)

	assert.NoError(t, drr.Close())
	errClosed := drr.Enqueue("sub-1", h.newEntry("j4", 1), 10)
	assert.ErrorIs(t, errClosed, ErrQueueClosed)

	_, errDeq := drr.Dequeue()
	assert.ErrorIs(t, errDeq, ErrQueueClosed)
}

// -----------------------------------------------------------------------------
// 3. FlowControlManager Tests
// -----------------------------------------------------------------------------

func TestFlowControlManager_IntegratedLifecycle(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	mgr := h.createFlowControlManager(100, 50, 50, 200)

	var dispatchedCount atomic.Int64
	sub := NewFuncSubscriber("sub-worker", func(ctx context.Context, entry *CallbackEntry) error {
		dispatchedCount.Add(1)
		return nil
	})

	errReg := mgr.RegisterSubscriber(sub, 50, 200, 100)
	assert.NoError(t, errReg)

	// Duplicate registration returns ErrSubscriberExists
	assert.ErrorIs(t, mgr.RegisterSubscriber(sub, 50, 200, 100), ErrSubscriberExists)

	// Submit event
	entry := h.newEntry("job-1", 1)
	errSub := mgr.Submit("sub-worker", entry, 10)
	assert.NoError(t, errSub)

	// Submit unknown subscriber
	errUnknown := mgr.Submit("sub-unknown", entry, 10)
	assert.ErrorIs(t, errUnknown, ErrSubscriberNotFound)

	// Dispatch next
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	dispatched, errDisp := mgr.DispatchNext(ctx)
	assert.NoError(t, errDisp)
	require.NotNil(t, dispatched)
	assert.Equal(t, "job-1", dispatched.JobID)
	assert.Equal(t, int64(1), dispatchedCount.Load())
}

// -----------------------------------------------------------------------------
// 4. Concurrent Producers and Consumers Stress Test (-race)
// -----------------------------------------------------------------------------

func (h *tokenBucketHarness) spawnDispatchers(ctx context.Context, mgr *FlowControlManager, count int, wg *sync.WaitGroup) {
	h.t.Helper()
	for d := 0; d < count; d++ {
		wg.Add(1)
		dispName := fmt.Sprintf("flow_dispatcher_%d", d)
		goroutinelabels.NewGoroutine(dispName, "concurrent dispatch loop").StartSimple(func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				entry, err := mgr.DispatchNext(ctx)
				if err != nil || entry == nil {
					return
				}
			}
		})
	}
}

func (h *tokenBucketHarness) spawnProducer(ctx context.Context, mgr *FlowControlManager, subID string, count int, wg *sync.WaitGroup) {
	h.t.Helper()
	wg.Add(1)
	prodName := fmt.Sprintf("flow_producer_%s", subID)
	goroutinelabels.NewGoroutine(prodName, "concurrent event submitter").StartSimple(func() {
		defer wg.Done()
		for itemIdx := 0; itemIdx < count; itemIdx++ {
			e := h.newEntry(fmt.Sprintf("%s-item-%d", subID, itemIdx), 1)
			for {
				if ctx.Err() != nil {
					return
				}
				err := mgr.Submit(subID, e, 1)
				if err == nil {
					break
				}
				if err == ErrCreditExhausted {
					time.Sleep(1 * time.Millisecond)
					continue
				}
				assert.NoError(h.t, err)
				break
			}
		}
	})
}

func TestFlowControl_ConcurrentStress_RaceSafety(t *testing.T) {
	t.Parallel()
	h := newTokenBucketHarness(t)
	mgr := h.createFlowControlManager(500, 1000, 100, 1000)

	const subCount = 4
	const itemsPerProducer = 50
	var totalReceived atomic.Int64

	for i := 0; i < subCount; i++ {
		name := fmt.Sprintf("tenant-%d", i)
		sub := NewFuncSubscriber(name, func(ctx context.Context, entry *CallbackEntry) error {
			totalReceived.Add(1)
			return nil
		})
		errReg := mgr.RegisterSubscriber(sub, 100, 1000, 500)
		assert.NoError(t, errReg)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	h.spawnDispatchers(ctx, mgr, 4, &wg)
	for p := 0; p < subCount; p++ {
		h.spawnProducer(ctx, mgr, fmt.Sprintf("tenant-%d", p), itemsPerProducer, &wg)
	}

	deadline := time.Now().Add(4 * time.Second)
	expectedTotal := int64(subCount * itemsPerProducer)
	for totalReceived.Load() < expectedTotal && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	errClose := mgr.Close()
	assert.NoError(t, errClose)
	wg.Wait()

	assert.Equal(t, expectedTotal, totalReceived.Load())
}

