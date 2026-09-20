package dispatch

import (
	"context"
	"sync"
	"testing"
)

func TestRunViaPool_WhenPoolNotStarted_RunsInline(t *testing.T) {
	StopGlobalPool() // ensure pool is not started so we test inline path
	ctx := context.Background()
	item := &WorkItem{OperationID: "test_1", OperationType: "test", ProjectRoot: "", Profile: "test"}
	called := false
	runner := func(context.Context) error {
		called = true
		return nil
	}
	err := RunViaPool(ctx, item, runner)
	if err != nil {
		t.Fatalf("RunViaPool: %v", err)
	}
	if !called {
		t.Error("runner was not called (expected inline run when pool not started)")
	}
}

func TestStartGlobalPool_RunViaPool_StopGlobalPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ensure no leftover pool from another test
	StopGlobalPool()

	StartGlobalPool(ctx)
	if !GlobalPoolStarted() {
		t.Fatal("GlobalPoolStarted() should be true after StartGlobalPool")
	}

	item := &WorkItem{OperationID: "pool_1", OperationType: "pool_test", ProjectRoot: "", Profile: "test"}
	var mu sync.Mutex
	runCount := 0
	runner := func(context.Context) error {
		mu.Lock()
		runCount++
		mu.Unlock()
		return nil
	}
	err := RunViaPool(ctx, item, runner)
	if err != nil {
		t.Fatalf("RunViaPool: %v", err)
	}
	mu.Lock()
	n := runCount
	mu.Unlock()
	if n != 1 {
		t.Errorf("runner called %d times, want 1", n)
	}

	StopGlobalPool()
	if GlobalPoolStarted() {
		t.Error("GlobalPoolStarted() should be false after StopGlobalPool")
	}
}
