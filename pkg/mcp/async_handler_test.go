package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type dummyHandler struct {
	delay time.Duration
}

func (d *dummyHandler) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	if d.delay > 0 {
		select {
		case <-time.After(d.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return "ok", nil
}

func TestAsyncHandler_LifetimeCounters(t *testing.T) {
	t.Parallel()
	var aNil *AsyncHandler
	actNil, totNil := aNil.GetAsyncHandlerStats()
	if actNil != 0 || totNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got (%d, %d)", actNil, totNil)
	}

	handler := &dummyHandler{delay: 50 * time.Millisecond}
	ah := NewAsyncHandler(handler, AsyncHandlerConfig{
		MaxConcurrent: 2,
		Timeout:       1 * time.Second,
	})

	aInit, tInit := ah.GetAsyncHandlerStats()
	if aInit != 0 || tInit != 0 {
		t.Fatalf("expected initial stats (0, 0), got (%d, %d)", aInit, tInit)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = ah.Handle(context.Background(), "test.method", nil)
	}()

	// Wait for handler to start
	time.Sleep(10 * time.Millisecond)

	if ah.GetActiveOperations() != 1 {
		t.Fatalf("expected 1 active operation, got %d", ah.GetActiveOperations())
	}

	wg.Wait()

	actAfter, totAfter := ah.GetAsyncHandlerStats()
	if actAfter != 0 || totAfter != 1 {
		t.Fatalf("expected stats (0, 1), got (%d, %d)", actAfter, totAfter)
	}
}

func TestAsyncHandler_CapacityLimit(t *testing.T) {
	t.Parallel()
	handler := &dummyHandler{delay: 100 * time.Millisecond}
	ah := NewAsyncHandler(handler, AsyncHandlerConfig{
		MaxConcurrent: 1,
		Timeout:       1 * time.Second,
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = ah.Handle(context.Background(), "slow.method", nil)
	}()

	time.Sleep(10 * time.Millisecond)

	// Second concurrent call should hit capacity limit
	_, err := ah.Handle(context.Background(), "fast.method", nil)
	if err == nil {
		t.Fatalf("expected error due to capacity limit")
	}

	wg.Wait()
}
