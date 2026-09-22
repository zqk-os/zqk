// BLI-STARTER-COMMUNITY-053 / PRI-STARTER-COMMUNITY-053 coverage elevation
package concurrency

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type extraLockMetrics struct {
	ops, contention, timeout float64
	maxWait                  time.Duration
}

func (m extraLockMetrics) GetObjectsPerSecond() float64 { return m.ops }
func (m extraLockMetrics) RecordLockWait(time.Duration) {}
func (m extraLockMetrics) RecordLockHold(time.Duration) {}
func (m extraLockMetrics) GetContentionRate() float64   { return m.contention }
func (m extraLockMetrics) GetTimeoutRate() float64      { return m.timeout }
func (m extraLockMetrics) GetMaxWaitTime() time.Duration {
	return m.maxWait
}

type extraLockLogger struct{}

func (extraLockLogger) Debug(string, ...LockField) {}
func (extraLockLogger) Warn(string, ...LockField)  {}

func TestExtraConcurrencyWrappersInterruptAndCeiling(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var rw sync.RWMutex
	log := extraLockLogger{}

	_ = DefaultLockTimeout()
	_ = CalculateLockTimeout(0, 0, time.Millisecond)
	_ = CalculateLockTimeout(100, 50, time.Second)

	if err := WithLock(&mu, "op", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithRLock(&rw, "op", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithLockCtx(&mu, ctx, "op", func() error { return errors.New("x") }); err == nil {
		t.Fatal("lock ctx err")
	}
	if err := WithRLockCtx(&rw, ctx, "op", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithLockLogger(&mu, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithRLockLogger(&rw, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithLockCtxLogger(&mu, ctx, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := WithRLockCtxLogger(&rw, ctx, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := RunInLockOrLog(&mu, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := RunInRLockOrLog(&rw, "op", log, func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	ops := []string{"cache_get", "unknown_op", "validator_stop", "concurrent_test"}
	metrics := []extraLockMetrics{
		{},
		{ops: 10, contention: 0.4, timeout: 0.3, maxWait: time.Second},
		{ops: 200, contention: 0.15, timeout: 0.08, maxWait: 10 * time.Millisecond},
		{ops: 50, contention: 0.07, timeout: 0.02},
	}
	for _, op := range ops {
		for _, m := range metrics {
			if err := WithLockTimeout(&mu, ctx, m, log, op, func() error { return nil }); err != nil {
				t.Fatalf("%s %+v: %v", op, m, err)
			}
			if err := WithRLockTimeout(&rw, ctx, m, log, op, func() error { return nil }); err != nil {
				t.Fatalf("rlock %s: %v", op, err)
			}
		}
	}

	chk := NewInterruptChecker(0)
	if err := chk.Check(ctx); err != nil {
		t.Fatal(err)
	}
	slow := NewInterruptChecker(time.Hour)
	if err := slow.Check(ctx); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	slow.lastCheck = time.Time{}
	if err := NewInterruptChecker(0).Check(canceled); err == nil {
		t.Fatal("interrupt canceled")
	}

	if err := WaitUnderGoroutineCeiling(ctx, 0, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, 5*time.Millisecond)
	defer waitCancel()
	_ = WaitUnderGoroutineCeiling(waitCtx, 1, time.Millisecond)

	nop := &NoOpOperationCallback{}
	nop.OnStart("id", nil)
	nop.OnProgress("id", 1, 2, "m")
	nop.OnComplete("id", "ok", time.Millisecond)
	nop.OnError("id", errors.New("e"))
	nop.OnCancel("id", "r")

	if err := AcquireRLockWithContext(ctx, &rw); err != nil {
		t.Fatal(err)
	}
	rw.RUnlock()
}
