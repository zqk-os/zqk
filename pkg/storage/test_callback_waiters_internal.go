//go:build !production

package storage

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// callbackWaiter provides deterministic waiting for callbacks to be invoked (same contract as pkg/testing.CallbackWaiter).
type callbackWaiter struct {
	mu          sync.Mutex
	invocations atomic.Int64
	waiters     []chan struct{}
}

func newCallbackWaiter() *callbackWaiter {
	return &callbackWaiter{
		waiters: make([]chan struct{}, 0),
	}
}

func (cw *callbackWaiter) Invoke() {
	count := cw.invocations.Add(1)
	cw.mu.Lock()
	defer cw.mu.Unlock()
	for _, ch := range cw.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	_ = count
}

func (cw *callbackWaiter) Count() int64 {
	return cw.invocations.Load()
}

func (cw *callbackWaiter) WaitForInvocation(ctx context.Context, timeout time.Duration) bool {
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

func (cw *callbackWaiter) WaitForCount(ctx context.Context, count int64, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return waitForCondition(
		timeoutCtx,
		func() bool { return cw.invocations.Load() >= count },
		10*time.Millisecond,
	)
}

// eventCollector collects events and provides deterministic waiting (same contract as pkg/testing.EventCollector).
type eventCollector struct {
	mu     sync.Mutex
	events []any
	notify chan struct{}
}

func newEventCollector() *eventCollector {
	return &eventCollector{
		events: make([]any, 0),
		notify: make(chan struct{}, 1),
	}
}

func (ec *eventCollector) Add(event any) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = append(ec.events, event)
	select {
	case ec.notify <- struct{}{}:
	default:
	}
}

func (ec *eventCollector) GetEvents() []any {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	result := make([]any, len(ec.events))
	copy(result, ec.events)
	return result
}

func (ec *eventCollector) WaitForEvents(ctx context.Context, count int, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		ec.mu.Lock()
		currentCount := len(ec.events)
		ec.mu.Unlock()

		if currentCount >= count {
			return true
		}

		select {
		case <-ec.notify:
		case <-timeoutCtx.Done():
			return false
		}
	}
}
