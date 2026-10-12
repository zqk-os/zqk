package callback

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type boundaryHarness struct {
	t *testing.T
}

func newBoundaryHarness(t *testing.T) *boundaryHarness {
	t.Helper()
	return &boundaryHarness{t: t}
}

func (h *boundaryHarness) mustNewLimiter(cap int64, rate float64) *TokenBucketLimiter {
	h.t.Helper()
	lim, err := NewTokenBucketLimiter(TokenBucketConfig{
		Capacity:   cap,
		RefillRate: rate,
	})
	require.NoError(h.t, err)
	h.t.Cleanup(func() {
		assert.NoError(h.t, lim.Close())
	})
	return lim
}

func TestTokenBucket_ZeroRefillRate_DrainsToEmpty(t *testing.T) {
	t.Parallel()
	h := newBoundaryHarness(t)
	lim := h.mustNewLimiter(10, 0)

	assert.True(t, lim.TryAcquire(10))
	assert.False(t, lim.TryAcquire(1))
	assert.Equal(t, int64(0), lim.AvailableTokens())
}

func TestTokenBucket_AcquireWithPreCanceledContext(t *testing.T) {
	t.Parallel()
	h := newBoundaryHarness(t)
	lim := h.mustNewLimiter(10, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := lim.Acquire(ctx, 1)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestTokenBucket_ExcessiveAcquireRequestsRejected(t *testing.T) {
	t.Parallel()
	h := newBoundaryHarness(t)
	lim := h.mustNewLimiter(50, 10)

	assert.False(t, lim.TryAcquire(51))
	assert.False(t, lim.TryAcquire(-1))
	assert.False(t, lim.TryAcquire(0))
	assert.Equal(t, int64(50), lim.AvailableTokens())
}

func TestDeficitRoundRobin_DeficitCapAtMax(t *testing.T) {
	t.Parallel()
	h := newBoundaryHarness(t)
	drr := NewDeficitRoundRobinQueue(DeficitRoundRobinConfig{
		DefaultQuantum:    100,
		DefaultMaxDeficit: 250,
		DefaultMaxDepth:   10,
	})
	h.t.Cleanup(func() {
		assert.NoError(h.t, drr.Close())
	})

	errReg := drr.RegisterSubscriber("capped-sub", 100, 250, 10)
	assert.NoError(t, errReg)

	// Enqueue item with cost 300 (larger than maxDeficit 250)
	entry := &CallbackEntry{JobID: "too-large", Timestamp: time.Now()}
	errEnq := drr.Enqueue("capped-sub", entry, 300)
	assert.NoError(t, errEnq)

	// Even after several rounds, deficit will cap at 250 and cannot satisfy 300
	item, errDeq := drr.Dequeue()
	assert.NoError(t, errDeq)
	assert.Nil(t, item)
}

func TestFlowControlManager_UnregisteredSubscriberRejection(t *testing.T) {
	t.Parallel()
	h := newBoundaryHarness(t)
	mgr, err := NewFlowControlManager(FlowControlConfig{
		Limiter: TokenBucketConfig{
			Capacity:   100,
			RefillRate: 100,
		},
		Queue: DeficitRoundRobinConfig{
			DefaultQuantum:  50,
			DefaultMaxDepth: 10,
		},
	})
	require.NoError(t, err)
	h.t.Cleanup(func() {
		assert.NoError(h.t, mgr.Close())
	})

	entry := &CallbackEntry{JobID: "ghost", Timestamp: time.Now()}
	errSubmit := mgr.Submit("nonexistent-subscriber", entry, 10)
	assert.ErrorIs(t, errSubmit, ErrSubscriberNotFound)
}
