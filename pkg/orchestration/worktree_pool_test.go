package orchestration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

func TestWorktreePoolAcquisitionAndRelease(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-worktree-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pool := NewWorktreePool(tempDir, 5)
	defer pool.Close()

	ctx := context.Background()

	handle1, err := pool.Acquire(ctx, "agent-1", 1*time.Minute)
	if err != nil {
		t.Fatalf("expected successful acquire, got err: %v", err)
	}
	if handle1.Path == "" {
		t.Errorf("handle path cannot be empty")
	}
	if pool.ActiveCount() != 1 {
		t.Errorf("expected active count 1, got %d", pool.ActiveCount())
	}

	handle2, err := pool.Acquire(ctx, "agent-2", 1*time.Minute)
	if err != nil {
		t.Fatalf("expected successful second acquire, got err: %v", err)
	}
	if handle2.ID == handle1.ID {
		t.Errorf("expected unique handles, got matching %s", handle1.ID)
	}
	if pool.ActiveCount() != 2 {
		t.Errorf("expected active count 2, got %d", pool.ActiveCount())
	}

	err = pool.Release(ctx, handle1.ID)
	if err != nil {
		t.Fatalf("expected successful release: %v", err)
	}
	if pool.ActiveCount() != 1 {
		t.Errorf("expected active count 1 after release, got %d", pool.ActiveCount())
	}
}

func TestWorktreePoolConcurrentAcquire(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-worktree-concurrent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	capacity := 10
	pool := NewWorktreePool(tempDir, capacity)
	defer pool.Close()

	var wg sync.WaitGroup
	ctx := context.Background()

	handles := make([]*WorktreeHandle, capacity)
	errorsList := make([]error, capacity)

	for i := 0; i < capacity; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("test", "worktree-acquire").StartSimple(func() {
			defer wg.Done()
			h, err := pool.Acquire(ctx, "agent-concurrent", 10*time.Second)
			handles[idx] = h
			errorsList[idx] = err
		})
	}
	wg.Wait()

	for i, err := range errorsList {
		if err != nil {
			t.Errorf("worker %d failed to acquire: %v", i, err)
		}
	}
	if pool.ActiveCount() != capacity {
		t.Errorf("expected active count %d, got %d", capacity, pool.ActiveCount())
	}
}

func TestWorktreeSweeperPruning(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-worktree-sweeper-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pool := NewWorktreePool(tempDir, 5)
	defer pool.Close()

	ctx := context.Background()

	// Acquire with short lease (already expired)
	handle, err := pool.Acquire(ctx, "agent-stale", -1*time.Second)
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	_ = handle

	if pool.ActiveCount() != 1 {
		t.Fatalf("expected 1 active worktree, got %d", pool.ActiveCount())
	}

	sweeper := NewWorktreeSweeper(pool)
	pruned, err := sweeper.Sweep(time.Now())
	if err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if pruned != 1 {
		t.Errorf("expected 1 pruned worktree, got %d", pruned)
	}
	if pool.ActiveCount() != 0 {
		t.Errorf("expected 0 active worktrees after sweep, got %d", pool.ActiveCount())
	}
}

// TestWorktreePool_BLI_STORAGE_WORKTREE_POOL_001 verifies ephemeral git worktree pool allocation and sweeper per BLI-STORAGE-WORKTREE-POOL-001.
func TestWorktreePool_BLI_STORAGE_WORKTREE_POOL_001(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zqk-worktree-evidence-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	pool := NewWorktreePool(tempDir, 3)
	defer pool.Close()

	ctx := context.Background()
	handle, err := pool.Acquire(ctx, "evidence-agent", 10*time.Minute)
	if err != nil {
		t.Fatalf("expected successful acquire for BLI-STORAGE-WORKTREE-POOL-001: %v", err)
	}
	if err := pool.Release(ctx, handle.ID); err != nil {
		t.Fatalf("expected release: %v", err)
	}
}
