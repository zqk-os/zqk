package graph_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/graph"
	"github.com/lanceman/zqk/pkg/graph/provider"
)

func TestGraphStateLocker(t *testing.T) {
	ctx := context.Background()

	// Mock connection pool
	prov := provider.NewMockGraphProvider()
	pool, err := prov.CreatePool(ctx, provider.ConnectionConfig{MaxConns: 1})
	if err != nil {
		t.Fatalf("Failed to create mock pool: %v", err)
	}

	owner1 := "worker-1"
	owner2 := "worker-2"
	resourceID := "test-resource-123"

	locker1 := graph.NewGraphStateLocker(pool, owner1)
	locker2 := graph.NewGraphStateLocker(pool, owner2)

	t.Run("Basic Lock and Release", func(t *testing.T) {
		release1, err := locker1.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		// Try acquiring with locker2, should fail with timeout
		_, err = locker2.Lock(ctx, resourceID, 2*time.Second, 500*time.Millisecond)
		if err != graph.ErrLockTimeout {
			t.Fatalf("Expected ErrLockTimeout, got: %v", err)
		}

		// Release the lock
		if err := release1(); err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}
	})

	t.Run("Process Death / TTL Expiry", func(t *testing.T) {
		// locker1 acquires lock with a very short TTL
		release1, err := locker1.Lock(ctx, resourceID, 100*time.Millisecond, 1*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		// Simulate process death (do not call release1)
		// Wait for TTL to expire
		time.Sleep(150 * time.Millisecond)

		// locker2 should now be able to steal the lock
		release2, err := locker2.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
		if err != nil {
			t.Fatalf("Locker2 failed to steal expired lock: %v", err)
		}

		if err := release2(); err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}

		// Calling release1 now might fail or succeed depending on implementation,
		// but locker2 successfully acquired it which is the antagonistic requirement.
		_ = release1()
	})

	t.Run("Context Cancellation Mid-Flight", func(t *testing.T) {
		// Acquire with locker1
		release1, err := locker1.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}

		cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		// Try acquiring with locker2, passing the context that will cancel
		_, err = locker2.Lock(cancelCtx, resourceID, 2*time.Second, 1*time.Second)
		if err != context.DeadlineExceeded && err != context.Canceled {
			t.Fatalf("Expected context timeout/cancellation, got: %v", err)
		}

		_ = release1()
	})

	t.Run("Concurrent Acquisition Race Condition", func(t *testing.T) {
		errCh := make(chan error, 2)

		// Both try to acquire at the same time
		go func() {
			release, err := locker1.Lock(ctx, resourceID, 2*time.Second, 500*time.Millisecond)
			if err == nil {
				time.Sleep(1 * time.Second)
				release()
			}
			errCh <- err
		}()

		go func() {
			release, err := locker2.Lock(ctx, resourceID, 2*time.Second, 500*time.Millisecond)
			if err == nil {
				time.Sleep(1 * time.Second)
				release()
			}
			errCh <- err
		}()

		err1 := <-errCh
		err2 := <-errCh

		// One should succeed, one should get a timeout
		if (err1 == nil && err2 == graph.ErrLockTimeout) || (err2 == nil && err1 == graph.ErrLockTimeout) {
			// Expected outcome
		} else {
			t.Fatalf("Expected one success and one timeout, got: err1=%v, err2=%v", err1, err2)
		}
	})
}
