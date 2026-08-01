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
	release2()

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
	release4()

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

				release()
			}
		}()
	}

	wg.Wait()

	if counter != 50 {
		t.Fatalf("Expected counter to be 50, but got %d (concurrent collision)", counter)
	}
}
