package semanticcache_test

import (
	"context"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/graph/provider"
	"github.com/lanceman/zqk/pkg/llm/semanticcache"
)

func TestCognitiveLiquidityPool(t *testing.T) {
	mockProv := provider.NewMockGraphProvider()
	graphPool, _ := mockProv.CreatePool(context.Background(), provider.ConnectionConfig{})

	pool := semanticcache.NewCognitiveLiquidityPool(graphPool, 1*time.Second)
	ctx := context.Background()

	// Test Store and Retrieve
	stateStr := "some shared context state"
	id, err := pool.Store(ctx, stateStr)
	if err != nil {
		t.Fatalf("unexpected error storing: %v", err)
	}

	if id == "" {
		t.Fatalf("expected non-empty ID")
	}

	retrieved, err := pool.Retrieve(ctx, id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retrieved != stateStr {
		t.Fatalf("expected %q, got %q", stateStr, retrieved)
	}

	// Test Not Found
	_, err = pool.Retrieve(ctx, "non-existent-id")
	if err == nil {
		t.Fatalf("expected error for non-existent ID")
	}

	// Test Expiration
	time.Sleep(1100 * time.Millisecond) // Wait for expiration

	_, err = pool.Retrieve(ctx, id)
	if err == nil {
		t.Fatalf("expected error for expired ID")
	}

	// Test Cleanup
	pool.Cleanup(ctx)

	// Add a new one, cleanup immediately (should not delete), then retrieve
	id2, _ := pool.Store(ctx, "another state")
	pool.Cleanup(ctx)

	_, err = pool.Retrieve(ctx, id2)
	if err != nil {
		t.Fatalf("unexpected error after cleanup for non-expired item: %v", err)
	}
}
