package graph

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStateLocking(t *testing.T) {
	locker := NewMemoryStateLocker()
	ctx := context.Background()
	resourceID := "node-123"

	// Test 1: Basic Lock and Release
	release1, err := locker.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Try acquiring again, should fail with timeout
	_, err = locker.Lock(ctx, resourceID, 2*time.Second, 500*time.Millisecond)
	if err != ErrLockTimeout {
		t.Fatalf("Expected ErrLockTimeout, got: %v", err)
	}

	// Release the lock
	err = release1()
	if err != nil {
		t.Fatalf("Failed to release lock: %v", err)
	}

	// Try acquiring again, should succeed
	release2, err := locker.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock after release: %v", err)
	}
	_ = release2()

	// Test 2: Orphan Lock Cleanup (Timeout gracefully)
	release3, err := locker.Lock(ctx, resourceID, 500*time.Millisecond, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}
	_ = release3 // intentionally do not release to simulate orphan

	// Wait for expiration
	time.Sleep(600 * time.Millisecond)

	// Should be able to acquire lock now as previous one expired
	release4, err := locker.Lock(ctx, resourceID, 2*time.Second, 1*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock after previous orphan expired: %v", err)
	}
	_ = release4()

	// Test 3: Concurrent Access
	var wg sync.WaitGroup
	var counter int

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				// Retry loop for the lock
				var release func() error
				var acqErr error
				for {
					release, acqErr = locker.Lock(ctx, resourceID, 2*time.Second, 100*time.Millisecond)
					if acqErr == nil {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				// Critical section
				curr := counter
				time.Sleep(2 * time.Millisecond) // Yield/simulate work
				counter = curr + 1

				_ = release()
			}
		}()
	}

	wg.Wait()

	if counter != 50 {
		t.Fatalf("Expected counter to be 50, but got %d (concurrent collision)", counter)
	}
}

func TestFencedStateLocking(t *testing.T) {
	locker := NewMemoryStateLocker().(FencedStateLocker)
	ctx := context.Background()
	resourceID := "fenced-res-42"

	// 1. Acquire lock with fence
	release, token1, err := locker.LockWithFence(ctx, resourceID, 200*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("LockWithFence: %v", err)
	}
	if token1 <= 0 {
		t.Fatalf("expected positive fence token, got %d", token1)
	}

	// Token must be valid while lease is active
	if !locker.ValidateFence(resourceID, token1) {
		t.Fatalf("expected token %d to be valid", token1)
	}

	// 2. Wait for lease to expire
	time.Sleep(250 * time.Millisecond)

	// Token must NOT be valid after expiration
	if locker.ValidateFence(resourceID, token1) {
		t.Fatalf("expected token %d to be invalid after expiration", token1)
	}
	_ = release()

	// 3. Acquire new lease - must receive strictly greater monotonic token
	release2, token2, err := locker.LockWithFence(ctx, resourceID, 200*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("LockWithFence second: %v", err)
	}
	defer release2()

	if token2 <= token1 {
		t.Fatalf("expected monotonic token token2 (%d) > token1 (%d)", token2, token1)
	}
	if !locker.ValidateFence(resourceID, token2) {
		t.Fatalf("expected token %d to be valid", token2)
	}
	// Old token remains invalid
	if locker.ValidateFence(resourceID, token1) {
		t.Fatalf("old token %d must not validate against new lease", token1)
	}
}

