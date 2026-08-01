package storage

import (
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

func TestCASOrphanCleanupQueue_LifetimeCounters(t *testing.T) {
	q, cancel := NewCASOrphanCleanupQueueForTest(pkgctx.NewSystemContext())
	defer cancel()

	initialStats := q.GetQueueStats()
	if initialStats.EnqueuedTotal != 0 {
		t.Fatalf("expected initial EnqueuedTotal=0, got %d", initialStats.EnqueuedTotal)
	}
	if initialStats.ProcessedTotal != 0 {
		t.Fatalf("expected initial ProcessedTotal=0, got %d", initialStats.ProcessedTotal)
	}

	// Enqueue requests
	testFile := t.TempDir() + "/orphan1.tmp"
	if err := q.EnqueueCleanup(testFile); err != nil {
		t.Fatalf("failed to enqueue cleanup: %v", err)
	}

	postEnqueueStats := q.GetQueueStats()
	if postEnqueueStats.EnqueuedTotal != 1 {
		t.Fatalf("expected EnqueuedTotal=1, got %d", postEnqueueStats.EnqueuedTotal)
	}

	// Simulate batch processing
	req := <-q.queue
	q.processBatch([]*orphanCleanupRequest{req})

	postProcessStats := q.GetQueueStats()
	if postProcessStats.ProcessedTotal != 1 {
		t.Fatalf("expected ProcessedTotal=1, got %d", postProcessStats.ProcessedTotal)
	}
}
