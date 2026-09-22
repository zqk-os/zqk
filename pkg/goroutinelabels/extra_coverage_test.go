package goroutinelabels

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoroutineLabels_ExtraCoverage(t *testing.T) {
	// 1. labels.go: StartNamedGoroutine & DoWithLabels
	doneNamed := make(chan struct{})
	StartNamedGoroutine("test_named_worker", "testing named worker", func() {
		close(doneNamed)
	})
	select {
	case <-doneNamed:
	case <-time.After(time.Second):
		t.Fatal("StartNamedGoroutine timed out")
	}

	var doRun1, doRun2 bool
	DoWithLabels("test_do", "test_purpose", func() {
		doRun1 = true
	})
	DoWithLabels("test_do_empty", "", func() {
		doRun2 = true
	})
	if !doRun1 || !doRun2 {
		t.Fatalf("expected DoWithLabels executions: doRun1=%v, doRun2=%v", doRun1, doRun2)
	}

	// 2. test_builder.go: NewTestGoroutine, Start, Wait, StartTestGoroutine, StartTestGoroutines
	tb := NewTestGoroutine("tb_worker", "testing tb")
	var tbRan atomic.Bool
	tb.Start(func() {
		tbRan.Store(true)
	})
	tb.Wait()
	if !tbRan.Load() {
		t.Fatal("NewTestGoroutine builder did not complete work")
	}

	var singleRan atomic.Bool
	wg1 := StartTestGoroutine("single_worker", "testing single", func() {
		singleRan.Store(true)
	})
	wg1.Wait()
	if !singleRan.Load() {
		t.Fatal("StartTestGoroutine did not complete")
	}

	var multiCount atomic.Int32
	wgMulti := StartTestGoroutines(t, 3, "multi_worker", "testing multi", func(id int) {
		multiCount.Add(1)
	})
	wgMulti.Wait()
	if multiCount.Load() != 3 {
		t.Fatalf("expected 3 runs, got %d", multiCount.Load())
	}

	// StartTestGoroutines count = 1 branch
	wgSingleMulti := StartTestGoroutines(t, 1, "single_prefix", "testing single prefix", func(id int) {})
	wgSingleMulti.Wait()

	// 3. builder.go: WithPreCleanup, WithPostCleanup, WithSignalOnExit, WithShutdownCheck, AsCleanup
	preRan := false
	cleanupRan := false
	postRan := false
	doneSignal := make(chan bool, 1)

	b := NewGoroutine("builder_full", "testing lifecycle cleanups")
	b.WithPreCleanup(func() {
		preRan = true
	}).WithCleanup(func() {
		cleanupRan = true
	}).WithPostCleanup(func() {
		postRan = true
	}).WithSignalOnExit(doneSignal).
		AsCleanup()

	b.StartSimple(func() {})

	select {
	case <-doneSignal:
	case <-time.After(time.Second):
		t.Fatal("WithSignalOnExit did not signal")
	}

	if !preRan || !cleanupRan || !postRan {
		t.Fatalf("cleanups failed to execute: pre=%v, clean=%v, post=%v", preRan, cleanupRan, postRan)
	}

	// WithShutdownCheck returning true should abort Start
	shutdownB := NewGoroutine("shutdown_worker", "testing shutdown skip")
	shutdownB.WithShutdownCheck(func() bool {
		return true
	})
	shutdownRan := false
	shutdownB.Start(func() error {
		shutdownRan = true
		return nil
	})
	time.Sleep(20 * time.Millisecond)
	if shutdownRan {
		t.Fatal("expected goroutine to not run when shutdownCheck returns true")
	}

	// 4. builder.go: StartWithResult
	resChan := make(chan any, 2)
	resultB := NewGoroutine("res_worker", "testing result")
	var resWg sync.WaitGroup
	resultB.WithWaitGroup(&resWg)
	fn := resultB.StartWithResult(resChan)

	fn(func() (any, error) {
		return "hello_res", nil
	})
	resWg.Wait()

	select {
	case res := <-resChan:
		if res != "hello_res" {
			t.Fatalf("expected hello_res, got %v", res)
		}
	default:
		t.Fatal("expected result from StartWithResult")
	}

	// StartWithResult with panic
	panicB := NewGoroutine("panic_res_worker", "testing panic result")
	panicRan := false
	panicB.WithPanicHandler(func(r any) {
		panicRan = true
	})
	fnPanic := panicB.StartWithResult(resChan)
	fnPanic(func() (any, error) {
		panic("boom")
	})

	select {
	case res := <-resChan:
		if err, ok := res.(error); !ok || err == nil {
			t.Fatalf("expected error from panic, got %v", res)
		}
	case <-time.After(time.Second):
		t.Fatal("expected panic error result")
	}
	if !panicRan {
		t.Fatal("expected panic handler to execute")
	}

	// StartWithResult with context cancelled
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ctxB := NewGoroutine("ctx_res_worker", "testing ctx cancel result")
	ctxB.WithContext(ctxCancelled)
	fnCtx := ctxB.StartWithResult(resChan)
	fnCtx(func() (any, error) {
		return "should not happen", nil
	})
	select {
	case res := <-resChan:
		if !errors.Is(res.(error), context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", res)
		}
	case <-time.After(time.Second):
		t.Fatal("expected context cancelled error")
	}

	// 5. pool.go: SetPoolCreationDeclinedNotifier & Name fallback
	var declinedLogged atomic.Bool
	SetPoolCreationDeclinedNotifier(func(name, purpose, reason string) {
		declinedLogged.Store(true)
	})
	notifyPoolCreationDeclined("declined_pool", "test_purpose", "exceeded")
	if !declinedLogged.Load() {
		t.Fatal("expected pool creation declined notifier to be called")
	}
}
