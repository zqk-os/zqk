package validation

import (
	"runtime"
	"testing"
	"time"
)

func TestValidationSemaphoreCapacity_FailFastCap(t *testing.T) {
	t.Parallel()
	got := validationSemaphoreCapacity(32)
	if got > 16 {
		t.Fatalf("validationSemaphoreCapacity(32)=%d exceeds fail-fast cap 16", got)
	}
	if runtime.NumCPU()*2 >= 16 && got != 16 {
		t.Fatalf("validationSemaphoreCapacity(32)=%d want 16 on this host", got)
	}
	if got < 4 {
		t.Fatalf("validationSemaphoreCapacity(32)=%d below floor 4", got)
	}
	got = validationSemaphoreCapacity(4)
	if got != 4 {
		t.Fatalf("validationSemaphoreCapacity(4)=%d want 4", got)
	}
}

func TestAwaitNestedValidation_TimeoutStillWaitsForDone(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	timeout := make(chan struct{})
	close(timeout)
	go func() {
		time.Sleep(25 * time.Millisecond)
		close(done)
	}()
	start := time.Now()
	if !awaitNestedValidation(done, timeout) {
		t.Fatal("expected timedOut=true")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("returned in %v; must wait for nested goroutine after timeout", elapsed)
	}
}

func TestAwaitNestedValidation_CompletesBeforeTimeout(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	timeout := make(chan struct{})
	close(done)
	if awaitNestedValidation(done, timeout) {
		t.Fatal("expected timedOut=false when done is already closed")
	}
}
