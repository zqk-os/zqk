package scheduler

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/circuitbreaker"
	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
)

// TestGlobalTestJobLimit_derivesFromHostAndStaysInBounds pins the sizing rule. The limit must leave
// the machine usable (never all logical CPUs) while still making progress on small hosts.
func TestGlobalTestJobLimit_derivesFromHostAndStaysInBounds(t *testing.T) {
	t.Parallel()
	got := globalTestJobLimit()
	if got < 1 {
		t.Fatalf("limit %d would stall every test job", got)
	}
	if cpus := runtime.NumCPU(); got >= cpus && cpus > 2 {
		t.Errorf("limit %d claims every one of %d logical CPUs; the point of the cap is to leave "+
			"headroom for the daemon and foreground work", got, cpus)
	}
	if got > 8 {
		t.Errorf("limit %d exceeds the documented ceiling of 8", got)
	}
	t.Logf("host has %d logical CPUs -> global test job limit %d, per-job parallelism %d",
		runtime.NumCPU(), got, concurrency.GetGlobalConcurrencyConfig().SchedulerTestJobParallelism)
}

// TestAcquireGlobalTestJobSlot_capsConcurrentHolders is the guard that matters: without it the
// scheduler had no global bound at all, and this asserts the bound actually binds rather than just
// existing as a config field nothing consults.
func TestAcquireGlobalTestJobSlot_capsConcurrentHolders(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	s := &Scheduler{
		logger:                       logger,
		packageConcurrencyLimiter:    circuitbreaker.NewConcurrencyLimiter(logger, 2*time.Second),
		globalTestConcurrencyLimiter: circuitbreaker.NewConcurrencyLimiter(logger, 2*time.Second),
	}

	limit := globalTestJobLimit()
	ctx := context.Background()

	var live, peak int64
	var wg sync.WaitGroup
	// Twice the limit, so the cap has to turn some away and queue them.
	for range limit * 2 {
		wg.Add(1)
		goroutinelabels.NewGoroutine("test-global-test-job-slot-acquire", "test acquiring global test job slot").
			StartSimple(func() {
				defer wg.Done()
				if err := s.acquireGlobalTestJobSlot(ctx); err != nil {
					return // waited past max-wait; the cap held, which is the behavior under test
				}
				defer s.releaseGlobalTestJobSlot()

				cur := atomic.AddInt64(&live, 1)
				for {
					old := atomic.LoadInt64(&peak)
					if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
						break
					}
				}
				time.Sleep(30 * time.Millisecond)
				atomic.AddInt64(&live, -1)
			})
	}
	// Deterministic wait: if the limiter ever fails to hand slots back, this test must fail with a
	// clear message rather than hang until the package timeout.
	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine("test-global-test-job-wait", "wait for concurrent slot acquisition to finish").
		StartSimple(func() {
			wg.Wait()
			close(waitDone)
		})
	select {
	case <-waitDone:
	case <-time.After(30 * time.Second):
		t.Fatalf("timed out waiting for test-job goroutines; %d still hold slots, so a slot is "+
			"leaking on release", atomic.LoadInt64(&live))
	}

	if peak > int64(limit) {
		t.Errorf("%d test jobs ran concurrently, above the global limit of %d; the cap is not binding",
			peak, limit)
	}
	if peak == 0 {
		t.Fatal("no job ever acquired a slot; this guard would be vacuous")
	}
	t.Logf("peak concurrent test jobs %d against limit %d", peak, limit)
}

// TestAcquireGlobalTestJobSlot_noLimiterDoesNotBlockDispatch documents the fail-open choice. A
// scheduler built without a limiter must keep dispatching; the gate protects the host, and refusing
// every test job when it is absent would be worse than not having it.
func TestAcquireGlobalTestJobSlot_noLimiterDoesNotBlockDispatch(t *testing.T) {
	t.Parallel()
	s := &Scheduler{logger: logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))}
	if err := s.acquireGlobalTestJobSlot(context.Background()); err != nil {
		t.Errorf("expected dispatch to proceed with no limiter configured, got %v", err)
	}
	s.releaseGlobalTestJobSlot() // must not panic
}

// TestAcquireGlobalTestJobSlot_waitsPastPackageMaxWaitUntilDispatchBudget is the
// TDE-1789763617048880000-b8016f74 regression: a slot held longer than the 2s package
// max-wait must still be acquired before the dispatch budget elapses, exactly once.
func TestAcquireGlobalTestJobSlot_waitsPastPackageMaxWaitUntilDispatchBudget(t *testing.T) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	packageMaxWait := 2 * time.Second
	dispatchBudget := 6 * time.Second
	s := &Scheduler{
		logger:                       logger,
		packageConcurrencyLimiter:    circuitbreaker.NewConcurrencyLimiter(logger, packageMaxWait),
		globalTestConcurrencyLimiter: circuitbreaker.NewConcurrencyLimiter(logger, dispatchBudget),
	}

	limit := globalTestJobLimit()
	ctx := context.Background()
	for range limit {
		if err := s.acquireGlobalTestJobSlot(ctx); err != nil {
			t.Fatalf("setup: filling global slots: %v", err)
		}
	}

	started := make(chan struct{})
	result := make(chan error, 1)
	goroutinelabels.NewGoroutine("test-global-slot-wait-past-package", "waiter past package max-wait").
		StartSimple(func() {
			close(started)
			result <- s.acquireGlobalTestJobSlot(ctx)
		})
	<-started

	select {
	case err := <-result:
		t.Fatalf("waiter returned after %v without a release (err=%v); global wait is still the package max-wait", packageMaxWait, err)
	case <-time.After(packageMaxWait + 400*time.Millisecond):
	}

	s.releaseGlobalTestJobSlot()

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("waiter failed after a slot was released within dispatch budget: %v", err)
		}
		s.releaseGlobalTestJobSlot()
	case <-time.After(dispatchBudget):
		t.Fatal("waiter did not acquire after a slot was released within the dispatch budget")
	}
}
