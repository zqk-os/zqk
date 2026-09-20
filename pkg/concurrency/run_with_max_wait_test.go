package concurrency

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRunWithMaxWait_CompletesBeforeTimeout(t *testing.T) {
	t.Parallel()
	var ran atomic.Bool
	RunWithMaxWait(func() {
		ran.Store(true)
	}, 5*time.Second)
	if !ran.Load() {
		t.Fatal("fn did not run")
	}
}

func TestRunWithMaxWait_ReturnsAfterTimeoutWhenBlocked(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	start := time.Now()
	RunWithMaxWait(func() {
		<-done // block until test closes
	}, 50*time.Millisecond)
	elapsed := time.Since(start)
	close(done)
	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("RunWithMaxWait returned after %v, want ~50ms", elapsed)
	}
}
