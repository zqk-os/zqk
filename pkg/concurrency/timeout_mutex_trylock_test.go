package concurrency

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// noTryLocker is a sync.Locker that does not implement TryLock.
type noTryLocker struct{ mu sync.Mutex }

func (l *noTryLocker) Lock()   { l.mu.Lock() }
func (l *noTryLocker) Unlock() { l.mu.Unlock() }

func TestAcquireLockWithContext_noTryLockFailsClosed(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := acquireLockWithContext(ctx, &noTryLocker{})
	if err == nil {
		t.Fatal("expected fail-closed error for Locker without TryLock")
	}
	if !strings.Contains(err.Error(), "TryLock") {
		t.Fatalf("error should mention TryLock, got %v", err)
	}
}

func TestWithLockTimeout_noTryLockFailsClosed(t *testing.T) {
	t.Parallel()

	err := WithLockTimeout(&noTryLocker{}, context.Background(), nil, nil, lockOpTestOperation, func() error {
		t.Fatal("callback should not run when lock acquisition is refused")
		return nil
	})
	if err == nil {
		t.Fatal("expected fail-closed error for Locker without TryLock")
	}
	if !strings.Contains(err.Error(), "TryLock") {
		t.Fatalf("error should mention TryLock, got %v", err)
	}
}
