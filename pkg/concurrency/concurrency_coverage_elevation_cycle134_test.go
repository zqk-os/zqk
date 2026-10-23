package concurrency

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type cycle134MockMetrics struct {
	opsRate        float64
	contentionRate float64
	timeoutRate    float64
	maxWaitTime    time.Duration
	recordedWaits  []time.Duration
	recordedHolds  []time.Duration
	mu             sync.Mutex
}

func (m *cycle134MockMetrics) GetObjectsPerSecond() float64 { return m.opsRate }
func (m *cycle134MockMetrics) GetContentionRate() float64   { return m.contentionRate }
func (m *cycle134MockMetrics) GetTimeoutRate() float64      { return m.timeoutRate }
func (m *cycle134MockMetrics) GetMaxWaitTime() time.Duration {
	return m.maxWaitTime
}

func (m *cycle134MockMetrics) RecordLockWait(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedWaits = append(m.recordedWaits, d)
}

func (m *cycle134MockMetrics) RecordLockHold(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedHolds = append(m.recordedHolds, d)
}

type cycle134MockLogger struct {
	mu       sync.Mutex
	debugs   []string
	warns    []string
	debugMap map[string][]LockField
	warnMap  map[string][]LockField
}

func newCycle134MockLogger() *cycle134MockLogger {
	return &cycle134MockLogger{
		debugMap: make(map[string][]LockField),
		warnMap:  make(map[string][]LockField),
	}
}

func (l *cycle134MockLogger) Debug(msg string, fields ...LockField) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.debugs = append(l.debugs, msg)
	l.debugMap[msg] = append(l.debugMap[msg], fields...)
}

func (l *cycle134MockLogger) Warn(msg string, fields ...LockField) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, msg)
	l.warnMap[msg] = append(l.warnMap[msg], fields...)
}

func (l *cycle134MockLogger) hasWarn(prefix string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.warns {
		if len(w) >= len(prefix) && w[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func (l *cycle134MockLogger) hasDebug(prefix string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, d := range l.debugs {
		if len(d) >= len(prefix) && d[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func TestCycle134_TimeoutMutex_AcquireTimeoutAndDeadlock(t *testing.T) {
	var mu sync.Mutex
	logger := newCycle134MockLogger()
	metrics := &cycle134MockMetrics{}

	mu.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := WithLockTimeout(&mu, ctx, metrics, logger, "contended_op", func() error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded timeout")
	assert.True(t, logger.hasWarn("Operation timeout while waiting for lock"))

	metrics.mu.Lock()
	assert.NotEmpty(t, metrics.recordedWaits)
	metrics.mu.Unlock()

	mu.Unlock()
}

func TestCycle134_TimeoutMutex_WaitThresholdAndExtendedHold(t *testing.T) {
	var mu sync.Mutex
	logger := newCycle134MockLogger()
	metrics := &cycle134MockMetrics{}

	mu.Lock()
	goroutinelabels.NewGoroutine("cycle134_test", "unlock after delay").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		mu.Unlock()
	})

	err := WithLockTimeout(&mu, context.Background(), metrics, logger, "delayed_op", func() error {
		time.Sleep(105 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasDebug("Lock acquired after wait"))
	assert.True(t, logger.hasDebug("Lock held for extended period"))

	metrics.mu.Lock()
	assert.NotEmpty(t, metrics.recordedWaits)
	assert.NotEmpty(t, metrics.recordedHolds)
	metrics.mu.Unlock()
}

func TestCycle134_TimeoutMutex_HoldExceedsTimeoutAndPanic(t *testing.T) {
	var mu sync.Mutex
	logger := newCycle134MockLogger()
	metrics := &cycle134MockMetrics{opsRate: 1000000}

	err := WithLockTimeout(&mu, context.Background(), metrics, logger, "progress_update", func() error {
		time.Sleep(15 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasWarn("Operation exceeded timeout while holding lock"))

	panicErr := WithLockTimeout(&mu, context.Background(), nil, nil, "panic_op", func() error {
		panic("simulated lock panic")
	})
	require.Error(t, panicErr)
	assert.Contains(t, panicErr.Error(), "panic in lock operation panic_op: simulated lock panic")
	assert.True(t, mu.TryLock(), "lock should be released after panic recovery")
	mu.Unlock()
}

func TestCycle134_TimeoutMutex_RLockTimeoutsAndPanics(t *testing.T) {
	var rw sync.RWMutex
	logger := newCycle134MockLogger()
	metrics := &cycle134MockMetrics{opsRate: 1000000}

	rw.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := WithRLockTimeout(&rw, ctx, metrics, logger, "contended_rop", func() error {
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeded timeout")
	assert.True(t, logger.hasWarn("Operation timeout while waiting for rlock"))
	rw.Unlock()

	rw.Lock()
	goroutinelabels.NewGoroutine("cycle134_test", "rw unlock after delay").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		rw.Unlock()
	})
	err = WithRLockTimeout(&rw, context.Background(), metrics, logger, "default", func() error {
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasDebug("RLock acquired after wait"))

	err = WithRLockTimeout(&rw, context.Background(), metrics, logger, "progress_update", func() error {
		time.Sleep(15 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasWarn("Operation exceeded timeout while holding rlock"))

	err = WithRLockTimeout(&rw, context.Background(), metrics, logger, "slow_rop", func() error {
		time.Sleep(105 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasDebug("RLock held for extended period"))

	panicErr := WithRLockTimeout(&rw, context.Background(), nil, nil, "panic_rop", func() error {
		panic("simulated rlock panic")
	})
	require.Error(t, panicErr)
	assert.Contains(t, panicErr.Error(), "panic in rlock operation panic_rop: simulated rlock panic")
	assert.True(t, rw.TryLock(), "rw lock should be released after panic recovery")
	rw.Unlock()
}

func TestCycle134_TimeoutMutex_ConvenienceWrappers(t *testing.T) {
	var rw sync.RWMutex
	logger := newCycle134MockLogger()
	ctx := context.Background()

	err := WithRWMutexCtxLogger(&rw, ctx, "write_cov", logger, true, func() error {
		return nil
	})
	require.NoError(t, err)

	err = WithRWMutexCtxLogger(&rw, ctx, "read_cov", logger, false, func() error {
		return nil
	})
	require.NoError(t, err)

	rw.Lock()
	cancelCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	acqErr := AcquireRLockWithContext(cancelCtx, &rw)
	require.Error(t, acqErr)
	rw.Unlock()
}

func TestCycle134_TimeoutMutex_CalculateLockTimeoutBranches(t *testing.T) {
	t1 := CalculateLockTimeout(1, 100, 5*time.Second)
	assert.Equal(t, 5*time.Second, t1)

	specOp := "spec_loader_check_cache"
	d1 := calculateLockTimeout(&cycle134MockMetrics{contentionRate: 0.35, timeoutRate: 0.25}, 1, specOp)
	assert.True(t, d1 > 0)

	d2 := calculateLockTimeout(&cycle134MockMetrics{contentionRate: 0.15, timeoutRate: 0.15}, 1, specOp)
	assert.True(t, d2 > 0)

	d3 := calculateLockTimeout(&cycle134MockMetrics{contentionRate: 0.08, timeoutRate: 0.08}, 1, specOp)
	assert.True(t, d3 > 0)

	d4 := calculateLockTimeout(&cycle134MockMetrics{maxWaitTime: 10 * time.Second}, 1, "custom_op")
	assert.True(t, d4 >= 10*time.Second)

	d5 := calculateLockTimeout(&cycle134MockMetrics{opsRate: 1000000}, 1, "progress_update")
	assert.Equal(t, 10*time.Millisecond, d5)

	d6 := calculateLockTimeout(&cycle134MockMetrics{contentionRate: 0.35}, 100000, "default")
	assert.Equal(t, 30*time.Second, d6)
}

func TestCycle134_WaitGroupManager_UncoveredRoutines(t *testing.T) {
	mgr := NewWaitGroupManager()

	wg1 := mgr.CreateGroupForGoroutine("g-goroutine", "op-goroutine")
	require.NotNil(t, wg1)

	gotWG := mgr.GetGroup("g-goroutine")
	assert.Same(t, wg1, gotWG)

	assert.Nil(t, mgr.GetGroup("non-existent-group"))

	mgr.Add("non-existent-group", 1)
	mgr.Done("non-existent-group")
	mgr.Wait("non-existent-group")

	completed := mgr.WaitWithTimeout("non-existent-group", time.Millisecond)
	assert.True(t, completed)

	err := mgr.WaitWithContext(context.Background(), "non-existent-group")
	require.NoError(t, err)

	mgr.DeleteGroup("g-goroutine")
	assert.Equal(t, 0, mgr.GroupCount())
}

func TestCycle134_WaitGroupManager_WaitWithContextSuccess(t *testing.T) {
	mgr := NewWaitGroupManager()
	mgr.CreateGroup("grp-ctx-succ", "op-succ")
	mgr.Add("grp-ctx-succ", 1)

	goroutinelabels.NewGoroutine("cycle134_test", "wg done after delay").StartSimple(func() {
		time.Sleep(10 * time.Millisecond)
		mgr.Done("grp-ctx-succ")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := mgr.WaitWithContext(ctx, "grp-ctx-succ")
	require.NoError(t, err)
}

func TestCycle134_WaitCondContext_EdgeBranches(t *testing.T) {
	var mu sync.Mutex
	cond := sync.NewCond(&mu)

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	errPre := WaitCondContext(canceledCtx, cond, &mu, func() bool { return false })
	require.ErrorIs(t, errPre, context.Canceled)

	errImmediate := WaitCondContext(context.Background(), cond, &mu, func() bool { return true })
	require.NoError(t, errImmediate)

	waitCtx, cancelWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelWait()
	errTimeout := WaitCondContext(waitCtx, cond, &mu, func() bool { return false })
	require.Error(t, errTimeout)

	ready := false
	goroutinelabels.NewGoroutine("cycle134_test", "cond broadcast after delay").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		mu.Lock()
		ready = true
		cond.Broadcast()
		mu.Unlock()
	})
	errReady := WaitCondContext(context.Background(), cond, &mu, func() bool { return ready })
	require.NoError(t, errReady)
}

func TestCycle134_GoroutineCeiling_SaturationAndDrop(t *testing.T) {
	ctx := context.Background()

	err := WaitUnderGoroutineCeiling(ctx, -1, time.Millisecond)
	require.NoError(t, err)

	err = WaitUnderGoroutineCeiling(ctx, 100000, time.Millisecond)
	require.NoError(t, err)

	cancelCtx, cancel := context.WithTimeout(ctx, 15*time.Millisecond)
	defer cancel()
	errBlocked := WaitUnderGoroutineCeiling(cancelCtx, 1, 5*time.Millisecond)
	require.Error(t, errBlocked)

	base := runtime.NumGoroutine()
	release := make(chan struct{})
	var started sync.WaitGroup
	started.Add(3)

	for i := 0; i < 3; i++ {
		goroutinelabels.NewGoroutine("cycle134_test", "block on release").StartSimple(func() {
			started.Done()
			<-release
		})
	}
	started.Wait()

	ceiling := base + 2
	goroutinelabels.NewGoroutine("cycle134_test", "release workers").StartSimple(func() {
		time.Sleep(15 * time.Millisecond)
		close(release)
	})

	errWait := WaitUnderGoroutineCeiling(ctx, ceiling, 5*time.Millisecond)
	require.NoError(t, errWait)
}

func TestCycle134_InterruptChecker_EdgeCases(t *testing.T) {
	chk := NewInterruptChecker(10 * time.Millisecond)

	err := chk.Check(context.Background())
	require.NoError(t, err)

	time.Sleep(15 * time.Millisecond)
	err = chk.Check(context.Background())
	require.NoError(t, err)

	time.Sleep(15 * time.Millisecond)
	activeCtx, cancelActive := context.WithCancel(context.Background())
	err = chk.Check(activeCtx)
	require.NoError(t, err)

	cancelActive()
	time.Sleep(15 * time.Millisecond)
	err = chk.Check(activeCtx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCycle134_Poll_IntervalClampingAndConditionSuccess(t *testing.T) {
	calls := 0
	ok := PollUntil(context.Background(), 0, func() bool {
		calls++
		return calls >= 2
	})
	assert.True(t, ok)

	ctx, cancel := context.WithCancel(context.Background())
	condCalls := 0
	goroutinelabels.NewGoroutine("cycle134_test", "cancel poll context").StartSimple(func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	})
	okCancel := PollUntil(ctx, 2*time.Millisecond, func() bool {
		condCalls++
		return ctx.Err() != nil
	})
	assert.True(t, okCancel)

	okTimeout := PollTimeout(15*time.Millisecond, 5*time.Millisecond, func() bool {
		return false
	})
	assert.False(t, okTimeout)
}

func TestCycle134_RunInLock_WithLoggerThresholds(t *testing.T) {
	var mu sync.Mutex
	var rw sync.RWMutex
	logger := newCycle134MockLogger()

	err := RunInLockWithLogger(&mu, "long_lock", logger, func() error {
		time.Sleep(105 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasDebug("Lock held for extended period"))

	err = RunInRLockWithLogger(&rw, "long_rlock", logger, func() error {
		time.Sleep(105 * time.Millisecond)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, logger.hasDebug("RLock held for extended period"))
}

func TestCycle134_RunWithMaxWait_TimeoutAndCompletion(t *testing.T) {
	completed := atomic.Bool{}
	RunWithMaxWait(func() {
		completed.Store(true)
	}, 50*time.Millisecond)
	assert.True(t, completed.Load())

	slowCompleted := atomic.Bool{}
	RunWithMaxWait(func() {
		time.Sleep(40 * time.Millisecond)
		slowCompleted.Store(true)
	}, 10*time.Millisecond)
	assert.False(t, slowCompleted.Load())
}

func TestCycle134_Config_OverridesAndDefaults(t *testing.T) {
	SetGlobalConcurrencyConfig(nil)

	cfg := GetGlobalConcurrencyConfig()
	require.NotNil(t, cfg)

	origValidator := cfg.ValidatorMaxWorkers
	origRouter := cfg.AsyncRouterMaxWorkers

	SetGlobalConcurrencyConfig(&ConcurrencyConfig{
		ValidatorMaxWorkers:   origValidator + 1,
		AsyncRouterMaxWorkers: origRouter + 1,
	})

	updated := GetGlobalConcurrencyConfig()
	assert.Equal(t, origValidator+1, updated.ValidatorMaxWorkers)
	assert.Equal(t, origRouter+1, updated.AsyncRouterMaxWorkers)

	SetGlobalConcurrencyConfig(&ConcurrencyConfig{
		ValidatorMaxWorkers:   origValidator,
		AsyncRouterMaxWorkers: origRouter,
	})

	defCfg := initDefaultConfig()
	require.NotNil(t, defCfg)
	assert.True(t, defCfg.ValidatorMaxWorkers >= 4)
	assert.True(t, defCfg.AsyncRouterMaxWorkers >= 2)

	jobs := defaultSchedulerMaxConcurrentTestJobs()
	assert.True(t, jobs >= 1 && jobs <= 8)
}

func TestCycle134_OperationCallback_MockCoverage(t *testing.T) {
	var cb OperationCallback = &NoOpOperationCallback{}
	cb.OnStart("op-1", map[string]any{"key": "val"})
	cb.OnProgress("op-1", 50, 100, "working")
	cb.OnComplete("op-1", "done", 10*time.Millisecond)
	cb.OnError("op-1", errors.New("err"))
	cb.OnCancel("op-1", "user requested")
	require.NotNil(t, cb)
}
