package circuitbreaker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

func TestCycle149_FixedWindowLimiter_Coverage(t *testing.T) {
	now := time.Now()
	nowFunc := func() time.Time { return now }

	limiter := NewFixedWindowLimiter(2, 100*time.Millisecond)
	limiter.SetNowFunc(nowFunc)

	assert.True(t, limiter.Allow("user1"))
	assert.True(t, limiter.Allow("user1"))
	assert.False(t, limiter.Allow("user1"))

	assert.True(t, limiter.Allow("user2"))

	now = now.Add(150 * time.Millisecond)
	assert.True(t, limiter.Allow("user1"))
}

func TestCycle149_FixedWindowLimiter_FallbackDefaults(t *testing.T) {
	limiter := NewFixedWindowLimiter(0, 0)
	assert.True(t, limiter.Allow("key1"))
}

func TestCycle149_TPMRateLimiter_TokenRequests(t *testing.T) {
	limiter := NewTPMRateLimiter(60000)

	ctx := context.Background()
	err := limiter.Wait(ctx, 0)
	require.NoError(t, err)

	err = limiter.Wait(ctx, -5)
	require.NoError(t, err)

	err = limiter.Wait(ctx, 60000)
	require.NoError(t, err)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	err = limiter.Wait(cancelCtx, 50000)
	assert.Error(t, err)
}

func TestCycle149_DefaultCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker()
	require.NoError(t, cb.AllowRequest())

	cb.RecordFailure()
	cb.RecordFailure()
	require.NoError(t, cb.AllowRequest())

	cb.RecordFailure()
	err := cb.AllowRequest()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit_open")

	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-10 * time.Second)
	cb.mu.Unlock()

	require.NoError(t, cb.AllowRequest()) // Transitions open -> half-open
	require.NoError(t, cb.AllowRequest()) // First probe allowed

	errProbe := cb.AllowRequest() // Second probe rejected while first is probing
	require.Error(t, errProbe)
	assert.Contains(t, errProbe.Error(), "circuit_half_open_probing")

	cb.RecordSuccess()
	require.NoError(t, cb.AllowRequest())
}

func TestCycle149_DefaultCircuitBreaker_FailureDecay(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.RecordFailure()
	cb.RecordFailure()

	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-35 * time.Second)
	cb.mu.Unlock()

	cb.RecordFailure()
	require.NoError(t, cb.AllowRequest())
}

func TestCycle149_DefaultCircuitBreaker_UnknownState(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.mu.Lock()
	cb.state = "invalid_state"
	cb.mu.Unlock()

	err := cb.AllowRequest()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown_circuit_state")
}

func TestCycle149_ConcurrencyLimiter_MergeLimitsAndCaps(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 2*time.Second)

	limiter.MergeLimits(nil)
	assert.False(t, limiter.ShouldLimit("pkg/test"))

	limits := map[string]int{
		"./pkg/storage": 50,
		"pkg/scheduler": 2,
		"pkg/empty":     0,
		"":              5,
	}
	limiter.MergeLimits(limits)

	limit, inUse := limiter.Snapshot("pkg/storage")
	assert.Equal(t, 32, limit)
	assert.Equal(t, 0, inUse)
	assert.True(t, limiter.ShouldLimit("pkg/storage"))

	limitsUpdate := map[string]int{
		"pkg/storage": -1,
	}
	limiter.MergeLimits(limitsUpdate)
	assert.False(t, limiter.ShouldLimit("pkg/storage"))
}

func TestCycle149_ConcurrencyLimiter_AcquireReleaseWaiters(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 500*time.Millisecond)

	limiter.SetLimit("pkg/custom", 1)
	ctx := context.Background()

	err := limiter.Acquire(ctx, "pkg/custom")
	require.NoError(t, err)

	limit, inUse := limiter.Snapshot("pkg/custom")
	assert.Equal(t, 1, limit)
	assert.Equal(t, 1, inUse)

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("test-waiter", "test").StartSimple(func() {
		defer wg.Done()
		wErr := limiter.Acquire(ctx, "pkg/custom")
		assert.NoError(t, wErr)
		limiter.Release("pkg/custom")
	})

	time.Sleep(50 * time.Millisecond)
	limiter.Release("pkg/custom")
	wg.Wait()
}

func TestCycle149_ConcurrencyLimiter_AcquireTimeout(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 50*time.Millisecond)
	limiter.SetLimit("pkg/slow", 1)

	ctx := context.Background()
	require.NoError(t, limiter.Acquire(ctx, "pkg/slow"))
	defer limiter.Release("pkg/slow")

	err := limiter.Acquire(ctx, "pkg/slow")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
}

func TestCycle149_ConcurrencyLimiter_AcquireContextCancel(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 2*time.Second)
	limiter.SetLimit("pkg/cancel", 1)

	ctx := context.Background()
	require.NoError(t, limiter.Acquire(ctx, "pkg/cancel"))
	defer limiter.Release("pkg/cancel")

	cancelCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("test-cancel", "test").StartSimple(func() {
		defer wg.Done()
		err := limiter.Acquire(cancelCtx, "pkg/cancel")
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
	})

	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestCycle149_ConcurrencyLimiter_MergeLimitsWithActiveSem(t *testing.T) {
	logger := logging.GetLoggerFromProfile("test")
	limiter := NewConcurrencyLimiter(logger, 2*time.Second)

	limiter.SetLimit("pkg/merge", 1)
	ctx := context.Background()
	require.NoError(t, limiter.Acquire(ctx, "pkg/merge"))
	defer limiter.Release("pkg/merge")

	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("waiter-merge", "test").StartSimple(func() {
		defer wg.Done()
		wErr := limiter.Acquire(ctx, "pkg/merge")
		assert.NoError(t, wErr)
		limiter.Release("pkg/merge")
	})

	time.Sleep(50 * time.Millisecond)

	// Update existing limit to higher value to wake waiter
	limiter.MergeLimits(map[string]int{"pkg/merge": 2})
	wg.Wait()

	// Update with same limit to hit prev == v branch
	limiter.MergeLimits(map[string]int{"pkg/merge": 2})

	// Delete limit with active sem to hit sem.limit = defaultMaxConcurrentPerPackage
	limiter.MergeLimits(map[string]int{"pkg/merge": 0})
}

func TestCycle149_ExtractPackagePath(t *testing.T) {
	pkg1 := ExtractPackagePathFromRunWrapperCommand("/bin/sh", []string{"-c", "go test ./pkg/circuitbreaker/..."})
	assert.Equal(t, "pkg/circuitbreaker/...", pkg1)

	pkg2 := ExtractPackagePathFromRunWrapperCommand("/bin/sh", []string{"./pkg/storage", "-v"})
	assert.Equal(t, "pkg/storage", pkg2)

	pkg3 := ExtractPackagePathFromRunWrapperCommand("/bin/sh", []string{"echo", "hello"})
	assert.Equal(t, "", pkg3)
}
