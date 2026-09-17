package sync

import (
	"context"
	"testing"
	"time"
)

func TestMemorySyncProtocol_Sync(t *testing.T) {
	p := NewMemorySyncProtocol()
	ctx := context.Background()

	op := SyncOperation{
		ID:        "op1",
		Type:      "create",
		TargetID:  "node1",
		Payload:   map[string]any{"key": "value"},
		Timestamp: time.Now(),
	}

	err := p.RegisterOperation(ctx, op)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	err = p.Sync(ctx)
	if err != nil {
		t.Fatalf("expected no error on sync, got %v", err)
	}

	// Add assertions checking length of operations if exposed, but they are private
}

func TestMemorySyncProtocol_AcquireConsensusLock(t *testing.T) {
	p := NewMemorySyncProtocol()
	ctx := context.Background()

	release, err := p.AcquireConsensusLock(ctx, "res1", time.Second*5)
	if err != nil {
		t.Fatalf("expected to acquire lock, got %v", err)
	}

	// Try to acquire again
	_, err = p.AcquireConsensusLock(ctx, "res1", time.Second*5)
	if err != ErrLockAcquisitionFailed {
		t.Fatalf("expected ErrLockAcquisitionFailed, got %v", err)
	}

	// Release lock
	err = release()
	if err != nil {
		t.Fatalf("expected no error on release, got %v", err)
	}

	// Acquire again should succeed
	_, err = p.AcquireConsensusLock(ctx, "res1", time.Second*5)
	if err != nil {
		t.Fatalf("expected to acquire lock after release, got %v", err)
	}
}
