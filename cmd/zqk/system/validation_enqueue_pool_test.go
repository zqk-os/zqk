package system

import (
	"fmt"
	"runtime"
	"testing"
)

func TestEnqueueValidationForObject_DoesNotSpawnOneGoroutinePerCall(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 80; i++ {
		EnqueueValidationForObject("/nonexistent-zqk-thread-cap", fmt.Sprintf("BLI-THREAD-CAP-%d", i), "backlog_item", "")
	}
	after := runtime.NumGoroutine()
	grew := after - before
	// Bounded pool (≤8 workers). One StartSimple per call used to grow by ~80.
	if grew > 24 {
		t.Fatalf("EnqueueValidationForObject spawned too many goroutines: before=%d after=%d grew=%d (want ≤24)", before, after, grew)
	}
}

func TestIncrementalValidationWorkerCount_Capped(t *testing.T) {
	n := incrementalValidationWorkerCount()
	if n < 1 || n > incrementalValidationMaxWorkers {
		t.Fatalf("worker count %d want 1..%d", n, incrementalValidationMaxWorkers)
	}
}
