package graph_test

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/storage/graph"
)

func setupMockPool(t *testing.T) provider.ConnectionPool {
	mockProv := provider.NewMockGraphProvider()
	ctx := context.Background()
	pool, err := mockProv.CreatePool(ctx, provider.ConnectionConfig{MaxConns: 5})
	if err != nil {
		t.Fatalf("Failed to create mock pool: %v", err)
	}
	return pool
}

func TestGraphLock_TryLock(t *testing.T) {
	pool := setupMockPool(t)
	ctx := context.Background()

	lock1 := graph.NewGraphLock(pool, "resource1", "owner1")
	lock2 := graph.NewGraphLock(pool, "resource1", "owner2")

	// owner1 acquires the lock
	acquired, err := lock1.TryLock(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !acquired {
		t.Fatalf("Expected to acquire lock, but failed")
	}
	if !lock1.IsLocked() {
		t.Fatalf("Expected lock1 to report IsLocked() == true")
	}

	// owner2 fails to acquire the same lock
	acquired2, err := lock2.TryLock(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if acquired2 {
		t.Fatalf("Expected owner2 to fail acquiring lock, but succeeded")
	}

	// owner1 unlocks
	err = lock1.Unlock(ctx)
	if err != nil {
		t.Fatalf("Expected no error on unlock, got: %v", err)
	}
	if lock1.IsLocked() {
		t.Fatalf("Expected lock1 to report IsLocked() == false after unlock")
	}

	// owner2 can now acquire
	acquired3, err := lock2.TryLock(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !acquired3 {
		t.Fatalf("Expected owner2 to acquire lock after unlock")
	}
}

func TestGraphLock_LockWithTimeout(t *testing.T) {
	pool := setupMockPool(t)
	ctx := context.Background()

	lock1 := graph.NewGraphLock(pool, "resource_timeout", "owner1")
	lock2 := graph.NewGraphLock(pool, "resource_timeout", "owner2")

	// owner1 acquires
	acquired, err := lock1.TryLock(ctx)
	if err != nil || !acquired {
		t.Fatalf("owner1 failed to acquire")
	}

	// owner2 tries to acquire with timeout (should fail)
	err = lock2.LockWithTimeout(ctx, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("Expected timeout error, got nil")
	}

	// owner1 unlocks in background
	goroutinelabels.NewGoroutine("storage_graph_test", "delayed lock release").StartSimple(func() {
		time.Sleep(50 * time.Millisecond)
		_ = lock1.Unlock(ctx)
	})

	// owner3 tries to acquire with longer timeout (should succeed)
	lock3 := graph.NewGraphLock(pool, "resource_timeout", "owner3")
	err = lock3.LockWithTimeout(ctx, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("Expected to acquire lock after owner1 unlocks, but got: %v", err)
	}
}
