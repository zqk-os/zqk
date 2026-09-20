package cas_test

import (
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"testing"
	"time"
)

// TestCASOrphanCleanupQueue_CoordinatorIntegration_EmptyQueue tests ProcessQueueIfIdle on a queue
// with no pending work. Uses an isolated queue instance — not GetGlobalCASOrphanCleanupQueue — so
// parallel tests cannot enqueue cleanup work that this call would process (flaky "got 1" failures).
func TestCASOrphanCleanupQueue_CoordinatorIntegration_EmptyQueue(t *testing.T) {
	ctx := context.Background()
	q, cancel := caspkg.NewCASOrphanCleanupQueueWithCancelForTest(ctx)
	defer cancel()

	cctx, ccancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer ccancel()

	processed, err := q.ProcessQueueIfIdle(cctx)
	if err != nil {
		t.Fatalf("ProcessQueueIfIdle() failed: %v", err)
	}

	if processed != 0 {
		t.Errorf("Expected 0 processed items, got %d", processed)
	}
}
