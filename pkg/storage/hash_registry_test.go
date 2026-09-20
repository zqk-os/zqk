package storage

import (
	"context"
	"testing"
	"time"
)

func TestHashRegistry_BasicLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	reg := NewHashRegistry(ctx, "test_kind", tmpDir)
	if reg == nil {
		t.Fatal("expected non-nil HashRegistry")
	}

	reg.SetHash("file1.yaml", "hash123")
	if h := reg.GetHash("file1.yaml"); h != "hash123" {
		t.Fatalf("expected hash123, got %s", h)
	}

	drainCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := reg.Drain(drainCtx); err != nil {
		t.Fatalf("drain failed: %v", err)
	}
}
