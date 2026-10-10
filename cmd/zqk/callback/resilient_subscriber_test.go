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
	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestResilientSubscriber_ConcurrencyLimiting(t *testing.T) {
	t.Parallel()

	maxConcurrent := 3
	var currentActive atomic.Int64
	var peakActive atomic.Int64

	inner := NewFuncSubscriber("concurrent_probe", func(ctx context.Context, entry *CallbackEntry) error {
		active := currentActive.Add(1)
		for {
			peak := peakActive.Load()
			if active <= peak || peakActive.CompareAndSwap(peak, active) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		currentActive.Add(-1)
		return nil
	})

	cfg := ResilientSubscriberConfig{
		MaxConcurrent:      maxConcurrent,
		ExecutionTimeout:   1 * time.Second,
		BlockTimeout:       500 * time.Millisecond,
		BackpressurePolicy: BackpressureBlock,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}

	resilient := NewResilientSubscriber(inner, cfg)

	var wg sync.WaitGroup
	totalRequests := 12
	errs := make(chan error, totalRequests)

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_concurrent_dispatch", "dispatch callback test entry").StartSimple(func() {
			defer wg.Done()
			entry := &CallbackEntry{Timestamp: time.Now()}
			err := resilient.Notify(context.Background(), entry)
			if err != nil {
				errs <- err
			}
		})
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	assert.LessOrEqual(t, peakActive.Load(), int64(maxConcurrent), "active concurrency must never exceed MaxConcurrent")
	_, _, total, failures := resilient.Metrics()
	assert.Equal(t, int64(totalRequests), total)
	assert.Equal(t, int64(0), failures)
}

func TestResilientSubscriber_AdaptiveBackpressureDrop(t *testing.T) {
	t.Parallel()

	blocker := make(chan struct{})
	inner := NewFuncSubscriber("slow_subscriber", func(ctx context.Context, entry *CallbackEntry) error {
		<-blocker
		return nil
	})

	// 1. Silent Drop
	cfgSilent := ResilientSubscriberConfig{
		MaxConcurrent:      1,
		ExecutionTimeout:   1 * time.Second,
		BackpressurePolicy: BackpressureDropSilent,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}
	resilientSilent := NewResilientSubscriber(inner, cfgSilent)

	// First request occupies the single concurrency slot
	goroutinelabels.NewGoroutine("test_occupy_slot_silent", "occupy subscriber concurrency slot").StartSimple(func() {
		if notifyErr := resilientSilent.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()}); notifyErr != nil {
			t.Logf("occupy slot silent error: %v", notifyErr)
		}
	})

	time.Sleep(10 * time.Millisecond)

	// Second request should be dropped silently
	err := resilientSilent.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()})
	assert.NoError(t, err, "silent drop must not return error")

	inFlightSilent, droppedSilent, totalSilent, failSilent := resilientSilent.Metrics()
	assert.GreaterOrEqual(t, inFlightSilent, int64(0))
	assert.GreaterOrEqual(t, failSilent, int64(0))
	assert.Equal(t, int64(1), droppedSilent)
	assert.Equal(t, int64(2), totalSilent)

	// 2. Error Drop
	cfgErr := ResilientSubscriberConfig{
		MaxConcurrent:      1,
		ExecutionTimeout:   1 * time.Second,
		BackpressurePolicy: BackpressureDropError,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}
	resilientErr := NewResilientSubscriber(inner, cfgErr)

	goroutinelabels.NewGoroutine("test_occupy_slot_error", "occupy subscriber concurrency slot").StartSimple(func() {
		if notifyErr := resilientErr.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()}); notifyErr != nil {
			t.Logf("occupy slot error: %v", notifyErr)
		}
	})

	time.Sleep(10 * time.Millisecond)

	// Second request should return explicit backpressure error
	err = resilientErr.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency limit reached")

	// Unblock inner goroutines
	close(blocker)
}

func TestResilientSubscriber_CircuitBreakerTripsAndRecovers(t *testing.T) {
	t.Parallel()

	var fail atomic.Bool
	fail.Store(true)

	inner := NewFuncSubscriber("failing_probe", func(ctx context.Context, entry *CallbackEntry) error {
		if fail.Load() {
			return errors.New("underlying network failure")
		}
		return nil
	})

	cfg := ResilientSubscriberConfig{
		MaxConcurrent:      4,
		ExecutionTimeout:   1 * time.Second,
		BackpressurePolicy: BackpressureDropError,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}

	resilient := NewResilientSubscriber(inner, cfg)

	// Cause 3 failures to trip the default circuit breaker
	for i := 0; i < 3; i++ {
		err := resilient.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "underlying network failure")
	}

	// 4th request must be rejected fail-fast by circuit breaker without reaching inner
	err := resilient.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker open")

	_, dropped, _, failures := resilient.Metrics()
	assert.Equal(t, int64(3), failures)
	assert.Equal(t, int64(1), dropped)
}

func TestResilientSubscriber_ExecutionTimeout(t *testing.T) {
	t.Parallel()

	inner := NewFuncSubscriber("hanging_probe", func(ctx context.Context, entry *CallbackEntry) error {
		select {
		case <-time.After(200 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	cfg := ResilientSubscriberConfig{
		MaxConcurrent:      2,
		ExecutionTimeout:   20 * time.Millisecond,
		BackpressurePolicy: BackpressureDropError,
		Breaker:            circuitbreaker.NewCircuitBreaker(),
	}

	resilient := NewResilientSubscriber(inner, cfg)

	err := resilient.Notify(context.Background(), &CallbackEntry{Timestamp: time.Now()})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || assert.Contains(t, err.Error(), "context deadline exceeded"))
}

func TestMultiSubscriberDispatcher_ResilientIntegration(t *testing.T) {
	t.Parallel()

	dispatcher := NewMultiSubscriberDispatcher(nil)

	var sub1Calls atomic.Int64
	var sub2Calls atomic.Int64

	sub1 := NewFuncSubscriber("healthy_sub", func(ctx context.Context, entry *CallbackEntry) error {
		sub1Calls.Add(1)
		return nil
	})

	sub2 := NewFuncSubscriber("faulty_sub", func(ctx context.Context, entry *CallbackEntry) error {
		sub2Calls.Add(1)
		return errors.New("sub2 permanent error")
	})

	r1 := dispatcher.RegisterResilient(sub1, DefaultResilientSubscriberConfig())
	r2 := dispatcher.RegisterResilient(sub2, DefaultResilientSubscriberConfig())
	require.NotNil(t, r1)
	require.NotNil(t, r2)

	entry := &CallbackEntry{Timestamp: time.Now()}

	// Dispatch multiple times; sub2 failures must not block sub1 from receiving events
	for i := 0; i < 5; i++ {
		dispatchErr := dispatcher.Dispatch(context.Background(), entry)
		assert.Error(t, dispatchErr)
	}

	assert.Equal(t, int64(5), sub1Calls.Load(), "healthy subscriber must receive all 5 events")
	assert.LessOrEqual(t, sub2Calls.Load(), int64(5), "faulty subscriber calls must be bounded by circuit breaker")

	inFlight2, dropped2, total2, failures2 := r2.Metrics()
	assert.GreaterOrEqual(t, inFlight2, int64(0))
	assert.Equal(t, int64(5), total2)
	assert.GreaterOrEqual(t, failures2, int64(3))
	assert.GreaterOrEqual(t, dropped2, int64(1), "circuit breaker must have shed calls once open")
}
