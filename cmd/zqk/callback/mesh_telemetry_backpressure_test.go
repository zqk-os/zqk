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

// closeThrottlerInCleanup ensures clean resource release without blank identifier assignment.
func closeThrottlerInCleanup(t *testing.T, throttler *MeshBackpressureThrottler) {
	t.Cleanup(func() {
		closeErr := throttler.Close()
		assert.NoError(t, closeErr)
	})
}

func setupTestThrottler(t *testing.T, policy BackpressurePolicy) *MeshBackpressureThrottler {
	t.Helper()
	th, err := NewMeshBackpressureThrottler(ThrottlerConfig{
		DefaultPolicy: policy,
	})
	require.NoError(t, err)
	closeThrottlerInCleanup(t, th)
	return th
}

func setupTestThrottlerWithAgg(t *testing.T, policy BackpressurePolicy) (*MeshBackpressureThrottler, *MeshTelemetryAggregator) {
	t.Helper()
	agg, err := NewMeshTelemetryAggregator(MeshTelemetryConfig{})
	require.NoError(t, err)

	th, err := NewMeshBackpressureThrottler(ThrottlerConfig{
		DefaultPolicy: policy,
		Aggregator:    agg,
	})
	require.NoError(t, err)
	closeThrottlerInCleanup(t, th)
	return th, agg
}

func registerTestWaitingPool(t *testing.T, throttler *MeshBackpressureThrottler, poolID string) {
	t.Helper()
	_, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyWait,
		InitialCredits: 0,
		MaxCredits:     10,
	})
	require.NoError(t, err)
}


// -----------------------------------------------------------------------------
// CRIT-1791759409830632000-8c68a2b5:
// Static Invariants: Layout, Bounded Sliding Window Buffer, and Metric Accumulator
// -----------------------------------------------------------------------------

func TestMeshTelemetry_StaticInvariants_LayoutAndBuffer(t *testing.T) {
	t.Parallel()

	// Verify SlidingWindowBuffer construction and configuration validation
	_, err := NewSlidingWindowBuffer(0, 10, nil)
	assert.ErrorIs(t, err, ErrInvalidConfiguration)

	_, err = NewSlidingWindowBuffer(10*time.Second, 0, nil)
	assert.ErrorIs(t, err, ErrInvalidConfiguration)

	swb, err := NewSlidingWindowBuffer(10*time.Second, 10, nil)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, swb.WindowDuration())
	assert.Equal(t, 10, swb.BucketCount())

	// Verify MeshTelemetryAggregator defaults
	agg, err := NewMeshTelemetryAggregator(MeshTelemetryConfig{})
	require.NoError(t, err)
	assert.Equal(t, DefaultMeshTelemetrySubscriberName, agg.Name())
	assert.NotNil(t, agg.WindowBuffer())
	assert.Equal(t, DefaultWindowDuration, agg.WindowBuffer().WindowDuration())
	assert.Equal(t, DefaultWindowBucketCount, agg.WindowBuffer().BucketCount())

	snap := agg.Snapshot()
	assert.Equal(t, int64(0), snap.TotalDispatched)
	assert.Equal(t, int64(0), snap.TotalReceived)
	assert.Equal(t, int64(0), snap.TotalDropped)
	assert.Equal(t, int64(0), snap.TotalErrors)
	assert.Equal(t, int64(0), snap.ActiveInFlight)
	assert.Equal(t, float64(0), snap.BufferUtilizationRatio)
	assert.Equal(t, time.Duration(0), snap.P50)
	assert.Equal(t, time.Duration(0), snap.P95)
	assert.Equal(t, time.Duration(0), snap.P99)
}

func TestMeshTelemetry_SlidingWindowBucketingAndPercentiles(t *testing.T) {
	t.Parallel()

	mockTime := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return mockTime }

	swb, err := NewSlidingWindowBuffer(10*time.Second, 10, clock)
	require.NoError(t, err)

	// Record events at T=0
	swb.Record(mockTime, 10, 10, 0, 10*time.Millisecond)
	swb.Record(mockTime, 10, 10, 0, 50*time.Millisecond)
	swb.Record(mockTime, 10, 10, 0, 100*time.Millisecond)

	m := swb.Aggregate(mockTime)
	assert.Equal(t, int64(30), m.TotalDispatched)
	assert.Equal(t, int64(30), m.TotalReceived)
	assert.Equal(t, int64(0), m.TotalDropped)
	assert.Equal(t, 3.0, m.ThroughputPerSec) // 30 events / 10 sec window
	assert.True(t, m.P50 > 0)
	assert.True(t, m.P95 >= m.P50)
	assert.True(t, m.P99 >= m.P95)

	// Advance time past the 10-second window
	futureTime := mockTime.Add(15 * time.Second)
	mFuture := swb.Aggregate(futureTime)
	assert.Equal(t, int64(0), mFuture.TotalDispatched)
	assert.Equal(t, float64(0), mFuture.ThroughputPerSec)
}

func TestMeshTelemetry_DimensionalAggregationAndSnapshots(t *testing.T) {
	t.Parallel()

	now := time.Now()
	agg, err := NewMeshTelemetryAggregator(MeshTelemetryConfig{
		Name: "test_aggregator",
		Clock: func() time.Time { return now },
	})
	require.NoError(t, err)

	// Record dispatches across topics, brokers, subscribers
	agg.RecordDispatch("orders", "broker-east-1", "worker-pool-1", 5*time.Millisecond)
	agg.RecordDispatch("orders", "broker-east-1", "worker-pool-2", 15*time.Millisecond)
	agg.RecordDispatch("payments", "broker-west-1", "worker-pool-1", 25*time.Millisecond)
	agg.RecordReceive("orders", "broker-east-1", "worker-pool-1")
	agg.RecordDrop("payments", "worker-pool-1", ReasonCreditExhausted)
	agg.RecordCreditDebt("worker-pool-1", 42)
	agg.RecordBufferUtilization(750, 1000)

	snap := agg.Snapshot()
	assert.Equal(t, "test_aggregator", snap.Name)
	assert.Equal(t, int64(3), snap.TotalDispatched)
	assert.Equal(t, int64(1), snap.TotalReceived)
	assert.Equal(t, int64(1), snap.TotalDropped)
	assert.Equal(t, int64(42), snap.CreditDebt)
	assert.Equal(t, 0.75, snap.BufferUtilizationRatio)

	require.Contains(t, snap.Topics, "orders")
	assert.Equal(t, int64(2), snap.Topics["orders"].TotalDispatched)
	assert.Equal(t, int64(1), snap.Topics["orders"].TotalReceived)

	require.Contains(t, snap.Topics, "payments")
	assert.Equal(t, int64(1), snap.Topics["payments"].TotalDispatched)
	assert.Equal(t, int64(1), snap.Topics["payments"].TotalDropped)

	require.Contains(t, snap.Brokers, "broker-east-1")
	assert.Equal(t, int64(2), snap.Brokers["broker-east-1"].TotalDispatched)

	require.Contains(t, snap.Subscribers, "worker-pool-1")
	assert.Equal(t, int64(2), snap.Subscribers["worker-pool-1"].TotalDispatched)
	assert.Equal(t, int64(1), snap.Subscribers["worker-pool-1"].TotalDropped)

	// Verify Notify() subscriber hook
	entry := &CallbackEntry{
		JobID:     "job-100",
		Timestamp: now.Add(-10 * time.Millisecond),
		Payload: map[string]any{
			MetricKeyTopic:      "telemetry-feed",
			MetricKeyBroker:     "broker-1",
			MetricKeySubscriber: "pool-a",
			MetricKeyStatus:     "success",
		},
	}
	err = agg.Notify(context.Background(), entry)
	require.NoError(t, err)

	snap2 := agg.Snapshot()
	assert.Equal(t, int64(4), snap2.TotalDispatched)
	require.Contains(t, snap2.Topics, "telemetry-feed")
}

// -----------------------------------------------------------------------------
// CRIT-1791759409830633000-1de1047f:
// Operational Dynamic Proof: Credit Replenishment, Adaptive Throttling & Burst Load
// -----------------------------------------------------------------------------

func TestMeshBackpressure_CreditDeductionAndReplenishment(t *testing.T) {
	t.Parallel()

	throttler := setupTestThrottler(t, PolicyReject)

	poolID := "sub-pool-alpha"
	pool, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyReject,
		InitialCredits: 10,
		MaxCredits:     20,
		DebtThreshold:  5,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(10), pool.Credits())

	ctx := context.Background()

	// Acquire 6 credits -> 4 remaining
	err = throttler.Acquire(ctx, poolID, 6)
	require.NoError(t, err)
	assert.Equal(t, int64(4), pool.Credits())
	assert.Equal(t, int64(1), pool.InFlight())

	// Replenish 10 credits -> capped at 14 (within MaxCredits 20)
	err = throttler.Replenish(poolID, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(14), pool.Credits())

	// Acknowledge event -> decreases in flight and replenishes
	err = throttler.Acknowledge(poolID, 2, 5*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pool.InFlight())
	assert.Equal(t, int64(16), pool.Credits())
}

func TestMeshBackpressure_PolicyWaitAndReplenishmentWakeup(t *testing.T) {
	t.Parallel()

	throttler := setupTestThrottler(t, PolicyWait)

	poolID := "sub-pool-waiting"
	registerTestWaitingPool(t, throttler, poolID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	acquiredCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_wait_acquire", "wait for credits in test").StartSimple(func() {
		acqErr := throttler.Acquire(ctx, poolID, 5)
		acquiredCh <- acqErr
	})

	// Ensure the goroutine is waiting
	time.Sleep(50 * time.Millisecond)
	select {
	case <-acquiredCh:
		t.Fatal("Acquire should have blocked waiting for credits")
	default:
	}

	// Replenish credits to unblock
	err := throttler.Replenish(poolID, 5)
	require.NoError(t, err)

	select {
	case waitErr := <-acquiredCh:
		require.NoError(t, waitErr)
	case <-time.After(1 * time.Second):
		t.Fatal("Acquire timed out waiting for replenishment signal")
	}
}

func TestMeshBackpressure_PolicyWait_ContextCancellation(t *testing.T) {
	t.Parallel()

	throttler := setupTestThrottler(t, PolicyWait)

	poolID := "sub-pool-cancel"
	registerTestWaitingPool(t, throttler, poolID)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := throttler.Acquire(ctx, poolID, 5)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestMeshBackpressure_PolicyDropOldest(t *testing.T) {
	t.Parallel()

	throttler, agg := setupTestThrottlerWithAgg(t, PolicyDropOldest)

	poolID := "sub-pool-lossy"
	pool, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyDropOldest,
		InitialCredits: 2,
		MaxCredits:     10,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Consume first 2 credits
	require.NoError(t, throttler.Acquire(ctx, poolID, 2))
	assert.Equal(t, int64(0), pool.Credits())
	assert.Equal(t, int64(0), pool.DroppedCount())

	// Next acquire has 0 credits available -> dropped without error
	require.NoError(t, throttler.Acquire(ctx, poolID, 1))
	assert.Equal(t, int64(1), pool.DroppedCount())

	snap := agg.Snapshot()
	assert.Equal(t, int64(1), snap.TotalDropped)
}

func TestMeshBackpressure_AdaptiveRateLimiter_AIMD(t *testing.T) {
	t.Parallel()

	throttler, err := NewMeshBackpressureThrottler(ThrottlerConfig{
		HighWatermark:          0.80,
		LowWatermark:           0.50,
		AdditiveIncrease:       100.0,
		MultiplicativeDecrease: 0.50,
	})
	require.NoError(t, err)
	closeThrottlerInCleanup(t, throttler)

	poolID := "adaptive-pool"
	pool, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:       poolID,
		RefillRate:   500.0,
		MinRateLimit: 50.0,
		MaxRateLimit: 2000.0,
	})
	require.NoError(t, err)

	initialRate := pool.RateLimit()
	assert.Equal(t, 500.0, initialRate)

	// Severe queue backlog: 90% utilization (>= 80% HighWatermark)
	// Multiplicative decrease: 500.0 * 0.5 = 250.0
	err = throttler.RecordDrain(poolID, 0, 900, 1000)
	require.NoError(t, err)
	assert.Equal(t, 250.0, pool.RateLimit())

	// Another severe backlog: 250.0 * 0.5 = 125.0
	err = throttler.RecordDrain(poolID, 0, 850, 1000)
	require.NoError(t, err)
	assert.Equal(t, 125.0, pool.RateLimit())

	// Drain down to low utilization: 20% utilization (<= 50% LowWatermark)
	// Additive increase: 125.0 + 100.0 = 225.0
	err = throttler.RecordDrain(poolID, 50, 200, 1000)
	require.NoError(t, err)
	assert.Equal(t, 225.0, pool.RateLimit())
}

func runBurstWorker(
	ctx context.Context,
	throttler *MeshBackpressureThrottler,
	agg *MeshTelemetryAggregator,
	poolID string,
	opsCount int,
	successfulOps *atomic.Int64,
	throttledOps *atomic.Int64,
) {
	for j := 0; j < opsCount; j++ {
		acqErr := throttler.Acquire(ctx, poolID, 1)
		if acqErr == nil {
			successfulOps.Add(1)
			agg.RecordDispatch("burst-topic", "broker-1", poolID, 2*time.Millisecond)
			ackErr := throttler.Acknowledge(poolID, 1, 2*time.Millisecond)
			if ackErr != nil {
				throttledOps.Add(1)
			}
		} else {
			throttledOps.Add(1)
		}
		drainErr := throttler.RecordDrain(poolID, 1, 100, 1000)
		if drainErr != nil {
			throttledOps.Add(1)
		}
	}
}

func runSnapshotLoop(agg *MeshTelemetryAggregator, stopSnap <-chan struct{}) {
	for {
		select {
		case <-stopSnap:
			return
		default:
			snapResult := agg.Snapshot()
			if snapResult.TotalDispatched < 0 {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func TestMeshBackpressure_ConcurrentBurstAndRaceDetector(t *testing.T) {
	t.Parallel()

	throttler, agg := setupTestThrottlerWithAgg(t, PolicyReject)

	poolID := "concurrent-burst-pool"
	_, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyReject,
		InitialCredits: 5000,
		MaxCredits:     10000,
		DebtThreshold:  1000,
	})
	require.NoError(t, err)

	concurrency := 20
	opsPerWorker := 100
	var workersWg sync.WaitGroup
	var snapWg sync.WaitGroup
	var successfulOps atomic.Int64
	var throttledOps atomic.Int64

	for i := 0; i < concurrency; i++ {
		workersWg.Add(1)
		goroutinelabels.NewGoroutine(fmt.Sprintf("concurrent_worker_%d", i), "concurrent burst stress test").StartSimple(func() {
			defer workersWg.Done()
			runBurstWorker(context.Background(), throttler, agg, poolID, opsPerWorker, &successfulOps, &throttledOps)
		})
	}

	stopSnap := make(chan struct{})
	snapWg.Add(1)
	goroutinelabels.NewGoroutine("concurrent_snapshotter", "snapshotting under burst").StartSimple(func() {
		defer snapWg.Done()
		runSnapshotLoop(agg, stopSnap)
	})

	workersWg.Wait()
	close(stopSnap)
	snapWg.Wait()

	assert.True(t, successfulOps.Load() > 0)
	snap := agg.Snapshot()
	assert.True(t, snap.TotalDispatched > 0)
}

// -----------------------------------------------------------------------------
// CRIT-1791759409830634000-af0e504b:
// Negative Boundaries & Defensive Rejection
// -----------------------------------------------------------------------------

func TestMeshBackpressure_NegativeBoundariesAndDefensiveRejection(t *testing.T) {
	t.Parallel()

	throttler := setupTestThrottler(t, PolicyReject)

	ctx := context.Background()

	// 1. Invalid pool registration
	_, err := throttler.RegisterPool(SubscriberPoolConfig{PoolID: ""})
	assert.ErrorIs(t, err, ErrInvalidConfiguration)

	// 2. Unregistered pool lookup
	_, err = throttler.GetPool("non-existent-pool")
	assert.ErrorIs(t, err, ErrSubscriberNotFound)

	// 3. Acquire on unregistered pool
	err = throttler.Acquire(ctx, "non-existent-pool", 1)
	assert.ErrorIs(t, err, ErrSubscriberNotFound)

	// 4. Invalid credit requests (zero or negative)
	_, err = throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         "test-defensive-pool",
		InitialCredits: 10,
		MaxCredits:     10,
		DebtThreshold:  5,
	})
	require.NoError(t, err)

	err = throttler.Acquire(ctx, "test-defensive-pool", 0)
	assert.ErrorIs(t, err, ErrInvalidCreditAmount)

	err = throttler.Acquire(ctx, "test-defensive-pool", -5)
	assert.ErrorIs(t, err, ErrInvalidCreditAmount)

	err = throttler.Replenish("test-defensive-pool", -1)
	assert.ErrorIs(t, err, ErrInvalidCreditAmount)

	// 5. Depleted credits rejection
	err = throttler.Acquire(ctx, "test-defensive-pool", 10)
	require.NoError(t, err)

	// Debt within threshold (requested 3 > available 0, debt 3 <= debtThreshold 5) -> ErrCreditExhausted
	err = throttler.Acquire(ctx, "test-defensive-pool", 3)
	assert.ErrorIs(t, err, ErrCreditExhausted)

	// 6. Debt threshold breach (requested 6 > debtThreshold 5) -> ErrDebtThresholdExceeded
	err = throttler.Acquire(ctx, "test-defensive-pool", 6)
	assert.ErrorIs(t, err, ErrDebtThresholdExceeded)

	// 7. Invalid buffer capacity in RecordDrain
	err = throttler.RecordDrain("test-defensive-pool", 1, 10, 0)
	assert.ErrorIs(t, err, ErrInvalidConfiguration)

	// 8. Nil CallbackEntry to Notify
	err = throttler.Notify(ctx, nil)
	assert.ErrorIs(t, err, ErrNilTelemetryEntry)
}

func TestMeshBackpressure_LifecycleCleanShutdownAndNoGoroutineLeaks(t *testing.T) {
	t.Parallel()

	throttler := setupTestThrottler(t, PolicyWait)

	poolID := "shutdown-pool"
	registerTestWaitingPool(t, throttler, poolID)

	waiterUnblocked := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_shutdown_waiter", "waiter unblocked on Close").StartSimple(func() {
		acqErr := throttler.Acquire(context.Background(), poolID, 5)
		waiterUnblocked <- acqErr
	})

	time.Sleep(50 * time.Millisecond)

	// Close the throttler
	err := throttler.Close()
	require.NoError(t, err)

	// Waiter must immediately unblock with ErrThrottlerClosed
	select {
	case waitErr := <-waiterUnblocked:
		assert.ErrorIs(t, waitErr, ErrThrottlerClosed)
	case <-time.After(1 * time.Second):
		t.Fatal("Waiter did not unblock upon Close()")
	}

	// Subsequent operations on closed throttler must fail
	err = throttler.Acquire(context.Background(), poolID, 1)
	assert.ErrorIs(t, err, ErrThrottlerClosed)

	// Idempotent Close()
	assert.NoError(t, throttler.Close())
}

func TestMeshController_UnifiedSubscriberIntegration(t *testing.T) {
	t.Parallel()

	throttler, agg := setupTestThrottlerWithAgg(t, PolicyReject)

	poolID := "unified-pool"
	_, err := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		InitialCredits: 5,
		MaxCredits:     10,
	})
	require.NoError(t, err)

	controller := NewMeshTelemetryBackpressureController(agg, throttler)
	assert.Equal(t, DefaultMeshControllerName, controller.Name())
	assert.Equal(t, agg, controller.Aggregator())
	assert.Equal(t, throttler, controller.Throttler())

	ctx := context.Background()

	// Notify valid entry
	entry := &CallbackEntry{
		JobID:     "ctrl-job-1",
		Timestamp: time.Now(),
		Payload: map[string]any{
			MetricKeyTopic:      "control-plane",
			MetricKeyBroker:     "broker-0",
			MetricKeySubscriber: poolID,
		},
	}

	err = controller.Notify(ctx, entry)
	require.NoError(t, err)

	snap := controller.Aggregator().Snapshot()
	assert.Equal(t, int64(1), snap.TotalDispatched)

	// Close controller
	ctrlCloseErr := controller.Close()
	assert.NoError(t, ctrlCloseErr)
}
