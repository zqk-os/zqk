package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestHydrationWorkerWaitTimeout_Constant(t *testing.T) {
	if hydrationWorkerWaitTimeout <= 0 {
		t.Errorf("expected positive hydrationWorkerWaitTimeout, got %v", hydrationWorkerWaitTimeout)
	}
	if hydrationWorkerWaitTimeout != 35*time.Second {
		t.Errorf("expected 35s hydrationWorkerWaitTimeout, got %v", hydrationWorkerWaitTimeout)
	}
}

func TestHydrationWorkerWaitTimeout_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(hydrationWorkerWaitTimeout):
		t.Fatal("context cancellation did not trigger")
	}
}
