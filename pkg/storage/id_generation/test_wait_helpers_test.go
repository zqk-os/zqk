package id_generation

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Duplicated from pkg/storage test helpers so this package does not import pkg/storage
// (storage imports id_generation — would cycle).

func waitForConditionWithTimeout(ctx context.Context, cond func() bool, timeout, interval time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if cond() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

type testCallbackWaiter struct {
	mu          sync.Mutex
	invocations atomic.Int64
	waiters     []chan struct{}
}

func newTestCallbackWaiter() *testCallbackWaiter {
	return &testCallbackWaiter{
		waiters: make([]chan struct{}, 0),
	}
}

func (cw *testCallbackWaiter) Invoke() {
	cw.invocations.Add(1)
	cw.mu.Lock()
	defer cw.mu.Unlock()
	for _, ch := range cw.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (cw *testCallbackWaiter) WaitForInvocation(ctx context.Context, timeout time.Duration) bool {
	if cw.invocations.Load() > 0 {
		return true
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ch := make(chan struct{}, 1)
	cw.mu.Lock()
	cw.waiters = append(cw.waiters, ch)
	cw.mu.Unlock()
	if cw.invocations.Load() > 0 {
		return true
	}
	select {
	case <-ch:
		return true
	case <-timeoutCtx.Done():
		return false
	}
}
