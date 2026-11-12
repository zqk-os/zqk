package community

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeadlockDetector_AcquireAndRelease(t *testing.T) {
	lm := NewProviderLockManager(2 * time.Second)
	ctx := context.Background()

	lock, err := lm.AcquireLock(ctx, "prov-1", "resource-A")
	if err != nil || lock == nil {
		t.Fatalf("expected successful lock acquisition, got: %v", err)
	}

	if lock.HolderID != "prov-1" || lock.ResourceID != "resource-A" {
		t.Fatalf("unexpected lock contents: %+v", lock)
	}

	// Release lock
	if err := lm.ReleaseLock("prov-1", "resource-A"); err != nil {
		t.Fatalf("expected clean release, got: %v", err)
	}

	// Double release should fail
	if err := lm.ReleaseLock("prov-1", "resource-A"); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("expected ErrLockNotHeld, got: %v", err)
	}
}

func TestDeadlockDetector_LeaseExpiryAndRenewal(t *testing.T) {
	// Short lease duration: 50ms
	lm := NewProviderLockManager(50 * time.Millisecond)
	ctx := context.Background()

	_, err := lm.AcquireLock(ctx, "prov-1", "resource-B")
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}

	// Renew lease
	if err := lm.RenewLease("prov-1", "resource-B", 200*time.Millisecond); err != nil {
		t.Fatalf("failed to renew: %v", err)
	}

	// Non-holder renew should fail
	if err := lm.RenewLease("prov-2", "resource-B", 100*time.Millisecond); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("expected ErrLockNotHeld, got: %v", err)
	}

	// Wait 250ms for renewed lease to expire
	time.Sleep(250 * time.Millisecond)

	// Now prov-2 should be able to acquire because lease expired
	lock2, err := lm.AcquireLock(ctx, "prov-2", "resource-B")
	if err != nil || lock2.HolderID != "prov-2" {
		t.Fatalf("expected prov-2 to acquire expired lease, got err: %v", err)
	}
}

func TestDeadlockDetector_CycleDetectionAndDeadlockRejection(t *testing.T) {
	lm := NewProviderLockManager(5 * time.Second)
	ctx := context.Background()

	// Prov-1 acquires Res-A
	_, err := lm.AcquireLock(ctx, "prov-1", "res-A")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Prov-2 acquires Res-B
	_, err = lm.AcquireLock(ctx, "prov-2", "res-B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Simulate prov-1 waiting on prov-2 (res-B)
	lm.waitGraph["prov-1"] = map[string]bool{"prov-2": true}

	// Now prov-2 attempting to acquire Res-A (held by prov-1) would create a cycle: prov-2 -> prov-1 -> prov-2
	_, err = lm.AcquireLock(ctx, "prov-2", "res-A")
	if !errors.Is(err, ErrDeadlockDetected) {
		t.Fatalf("expected ErrDeadlockDetected, got: %v", err)
	}
}

func TestDeadlockDetector_ContextCancellation(t *testing.T) {
	lm := NewProviderLockManager(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := lm.AcquireLock(ctx, "prov-3", "res-C")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
