package callback

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestMeshTelemetry_BoundaryConfigurations(t *testing.T) {
	// Zero or negative window duration
	bufZeroWindow, errZeroWindow := NewSlidingWindowBuffer(0, 10, nil)
	assert.Equal(t, ErrInvalidConfiguration, errZeroWindow)
	assert.Nil(t, bufZeroWindow)

	bufNegWindow, errNegWindow := NewSlidingWindowBuffer(-1*time.Second, 10, nil)
	assert.Equal(t, ErrInvalidConfiguration, errNegWindow)
	assert.Nil(t, bufNegWindow)

	// Zero or negative bucket count
	bufZeroBuckets, errZeroBuckets := NewSlidingWindowBuffer(10*time.Second, 0, nil)
	assert.Equal(t, ErrInvalidConfiguration, errZeroBuckets)
	assert.Nil(t, bufZeroBuckets)

	bufNegBuckets, errNegBuckets := NewSlidingWindowBuffer(10*time.Second, -5, nil)
	assert.Equal(t, ErrInvalidConfiguration, errNegBuckets)
	assert.Nil(t, bufNegBuckets)

	// Single bucket edge case
	bufSingle, errSingle := NewSlidingWindowBuffer(1*time.Second, 1, nil)
	require.NoError(t, errSingle)
	require.NotNil(t, bufSingle)
	assert.Equal(t, 1, bufSingle.BucketCount())
	assert.Equal(t, 1*time.Second, bufSingle.WindowDuration())
}

func TestMeshBackpressure_CreditBoundaryCases(t *testing.T) {
	throttler, initErr := NewMeshBackpressureThrottler(ThrottlerConfig{
		DefaultPolicy: PolicyReject,
	})
	require.NoError(t, initErr)
	defer func() {
		closeErr := throttler.Close()
		assert.NoError(t, closeErr)
	}()

	poolID := "boundary-pool"
	pool, regErr := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyReject,
		InitialCredits: 10,
		MaxCredits:     20,
	})
	require.NoError(t, regErr)

	ctx := context.Background()

	// Zero credit acquisition
	errZero := throttler.Acquire(ctx, poolID, 0)
	assert.Equal(t, ErrInvalidCreditAmount, errZero)

	// Negative credit acquisition
	errNeg := throttler.Acquire(ctx, poolID, -5)
	assert.Equal(t, ErrInvalidCreditAmount, errNeg)

	// Replenish with zero or negative credits
	errReplZero := throttler.Replenish(poolID, 0)
	assert.Equal(t, ErrInvalidCreditAmount, errReplZero)

	errReplNeg := throttler.Replenish(poolID, -2)
	assert.Equal(t, ErrInvalidCreditAmount, errReplNeg)

	// Replenishment beyond MaxCredits ceiling is clamped at MaxCredits
	require.NoError(t, throttler.Replenish(poolID, 100))
	assert.Equal(t, int64(20), pool.Credits())

	// Nonexistent subscriber pool queries
	errMissingRepl := throttler.Replenish("ghost-pool", 5)
	assert.Equal(t, ErrSubscriberNotFound, errMissingRepl)
}

func TestMeshBackpressure_ConcurrentSaturationStress(t *testing.T) {
	throttler, initErr := NewMeshBackpressureThrottler(ThrottlerConfig{
		DefaultPolicy: PolicyReject,
	})
	require.NoError(t, initErr)
	defer func() {
		closeErr := throttler.Close()
		assert.NoError(t, closeErr)
	}()

	poolID := "stress-pool"
	_, regErr := throttler.RegisterPool(SubscriberPoolConfig{
		PoolID:         poolID,
		Policy:         PolicyReject,
		InitialCredits: 50,
		MaxCredits:     50,
	})
	require.NoError(t, regErr)

	var wg sync.WaitGroup
	ctx := context.Background()

	for i := 0; i < 30; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("test_mesh_boundary_worker", "concurrent mesh credit acquisition stress").StartSimple(func() {
			defer wg.Done()
			workerCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
			defer cancel()

			acqErr := throttler.Acquire(workerCtx, poolID, 2)
			if acqErr == nil {
				replenishErr := throttler.Replenish(poolID, 2)
				assert.NoError(t, replenishErr)
			} else {
				assert.Equal(t, ErrCreditExhausted, acqErr, fmt.Sprintf("worker %d unexpected error", workerID))
			}
		})
	}
	wg.Wait()
}
