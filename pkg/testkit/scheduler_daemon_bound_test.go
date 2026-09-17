package testkit

import (
	"context"
	"testing"
	"time"
)

func TestDefaultSchedulerDaemonBounds_arePositiveAndOrdered(t *testing.T) {
	if DefaultMaxCLISchedulerDaemonLifetime <= 0 {
		t.Fatal("DefaultMaxCLISchedulerDaemonLifetime must be > 0")
	}
	if DefaultMaxInProcessSchedulerLifetime <= 0 {
		t.Fatal("DefaultMaxInProcessSchedulerLifetime must be > 0")
	}
	// CLI detached daemons get a slightly larger ceiling than in-process Start loops.
	if DefaultMaxCLISchedulerDaemonLifetime < DefaultMaxInProcessSchedulerLifetime {
		t.Fatalf("CLI bound %v should be >= in-process bound %v",
			DefaultMaxCLISchedulerDaemonLifetime, DefaultMaxInProcessSchedulerLifetime)
	}
}

func TestBoundSchedulerStartContext_cancelsAfterMax(t *testing.T) {
	ctx, cancel := BoundSchedulerStartContext(t, context.Background(), 20*time.Millisecond)
	defer cancel()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.DeadlineExceeded {
			t.Fatalf("want DeadlineExceeded, got %v", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context did not cancel within bound")
	}
}

func TestWaitChanOrBoundWithCap_prefersDone(t *testing.T) {
	done := make(chan struct{})
	close(done)
	bound, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := WaitChanOrBoundWithCap(done, bound, 50*time.Millisecond); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestWaitChanOrBoundWithCap_capBeforeBound(t *testing.T) {
	done := make(chan struct{})
	bound, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	err := WaitChanOrBoundWithCap(done, bound, 25*time.Millisecond)
	if err != context.DeadlineExceeded {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("cap wait took too long: %v", time.Since(start))
	}
}
