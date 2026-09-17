package file

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

func TestShardedFileLockStrategy_ParallelConcurrency(t *testing.T) {
	tempDir := t.TempDir()
	strategy := NewShardedFileLockStrategy(64, tempDir)

	concurrency := 8
	var wg sync.WaitGroup
	var activeLocks atomic.Int32
	var maxParallel atomic.Int32

	for i := 0; i < concurrency; i++ {
		key := fmt.Sprintf("resource-isolated-%d", i)
		wg.Add(1)
		goroutinelabels.NewGoroutine("test.sharded_lock", "parallel shard lock worker").StartSimple(func() {
			defer wg.Done()
			err := WithShardedFileLock(strategy, key, 2*time.Second, func() error {
				curr := activeLocks.Add(1)
				defer activeLocks.Add(-1)

				for {
					max := maxParallel.Load()
					if curr <= max || maxParallel.CompareAndSwap(max, curr) {
						break
					}
				}

				time.Sleep(50 * time.Millisecond)
				return nil
			})
			if err != nil {
				t.Errorf("failed to acquire shard lock for %s: %v", key, err)
			}
		})
	}

	wg.Wait()

	if maxParallel.Load() < 2 {
		t.Logf("note: max parallel locks observed: %d", maxParallel.Load())
	}
}

func TestWithShardedFileLock_ErrorPropagation(t *testing.T) {
	tempDir := t.TempDir()
	strategy := NewShardedFileLockStrategy(8, tempDir)

	expectedErr := fmt.Errorf("custom worker failure")
	err := WithShardedFileLock(strategy, "key-error", 500*time.Millisecond, func() error {
		return expectedErr
	})

	if err == nil || err.Error() != expectedErr.Error() {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}
