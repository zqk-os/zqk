package mcp

import (
	"context"
	"testing"
	"time"
)

func TestProcessGroupManager_IsShuttingDown(t *testing.T) {
	ctx := context.Background()
	pgm := NewProcessGroupManager(ctx, 1*time.Second)

	if pgm.IsShuttingDown() {
		t.Fatalf("expected IsShuttingDown to be false initially")
	}

	if err := pgm.Shutdown("test shutdown"); err != nil {
		t.Fatalf("unexpected error during shutdown: %v", err)
	}

	if !pgm.IsShuttingDown() {
		t.Fatalf("expected IsShuttingDown to be true after Shutdown")
	}

	// Idempotent second shutdown call
	if err := pgm.Shutdown("second shutdown"); err != nil {
		t.Fatalf("unexpected error on second shutdown: %v", err)
	}
}

func TestProcessGroupManager_SpawnGoroutine(t *testing.T) {
	ctx := context.Background()
	pgm := NewProcessGroupManager(ctx, 1*time.Second)

	done := make(chan struct{})
	var innerErr error
	_, unreg := pgm.SpawnGoroutine("g1", "test_goroutine", "test description", false, func(ctx context.Context) {
		innerErr = ctx.Err()
		close(done)
	})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("goroutine did not execute in time")
	}

	if innerErr != nil {
		t.Fatalf("expected active goroutine context inside fn, got err: %v", innerErr)
	}

	unreg()
}

func TestProcessGroupManager_SubprocessRegistration(t *testing.T) {
	ctx := context.Background()
	pgm := NewProcessGroupManager(ctx, 1*time.Second)

	killed := false
	killFunc := func() error {
		killed = true
		return nil
	}

	pgm.RegisterSubprocess("p1", "test_proc", "desc", 1234, false, killFunc)
	pgm.UnregisterSubprocess("p1")

	if err := pgm.Shutdown("cleanup"); err != nil {
		t.Fatalf("unexpected shutdown error: %v", err)
	}

	if killed {
		t.Fatalf("unregistered process should not be killed on shutdown")
	}
}

func TestProcessGroupManager_LifetimeCounters(t *testing.T) {
	var pgmNil *ProcessGroupManager
	gNil, sNil := pgmNil.GetProcessGroupStats()
	if gNil != 0 || sNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got (%d, %d)", gNil, sNil)
	}

	ctx := context.Background()
	pgm := NewProcessGroupManager(ctx, 1*time.Second)
	gInit, sInit := pgm.GetProcessGroupStats()
	if gInit != 0 || sInit != 0 {
		t.Fatalf("expected initial stats (0, 0), got (%d, %d)", gInit, sInit)
	}

	_, unreg := pgm.SpawnGoroutine("g1", "test_g", "desc", false, func(ctx context.Context) {})
	unreg()

	pgm.RegisterSubprocess("p1", "test_p", "desc", 123, false, nil)
	pgm.UnregisterSubprocess("p1")

	gAfter, sAfter := pgm.GetProcessGroupStats()
	if gAfter != 1 || sAfter != 1 {
		t.Fatalf("expected stats (1, 1), got (%d, %d)", gAfter, sAfter)
	}
}
