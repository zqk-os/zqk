package testing

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// CallbackWaiter provides deterministic waiting for callbacks to be invoked
type CallbackWaiter struct {
	mu          sync.Mutex
	invocations atomic.Int64
	waiters     []chan struct{}
}

// NewCallbackWaiter creates a new callback waiter
func NewCallbackWaiter() *CallbackWaiter {
	return &CallbackWaiter{
		waiters: make([]chan struct{}, 0),
	}
}

// Invoke marks that the callback was invoked
func (cw *CallbackWaiter) Invoke() {
	count := cw.invocations.Add(1)
	cw.mu.Lock()
	defer cw.mu.Unlock()
	// Notify all waiters
	for _, ch := range cw.waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	_ = count // Avoid unused variable warning
}

// Count returns the number of invocations
func (cw *CallbackWaiter) Count() int64 {
	return cw.invocations.Load()
}

// WaitForInvocation waits for the callback to be invoked at least once
// Returns true if callback was invoked, false if timeout
// ctx: parent context from command entry point (should not be created here)
func (cw *CallbackWaiter) WaitForInvocation(ctx context.Context, timeout time.Duration) bool {
	// Fast path: already invoked
	if cw.invocations.Load() > 0 {
		return true
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ch := make(chan struct{}, 1)
	cw.mu.Lock()
	cw.waiters = append(cw.waiters, ch)
	cw.mu.Unlock()

	// Check again after adding to waiters (race condition)
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

// WaitForCount waits for the callback to be invoked a specific number of times
// ctx: parent context from command entry point (should not be created here)
func (cw *CallbackWaiter) WaitForCount(ctx context.Context, count int64, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return WaitForCondition(
		timeoutCtx,
		func() bool { return cw.invocations.Load() >= count },
		10*time.Millisecond,
	)
}

// AtomicCounter provides a thread-safe counter for callback tracking
type AtomicCounter struct {
	value atomic.Int64
}

// NewAtomicCounter creates a new atomic counter
func NewAtomicCounter() *AtomicCounter {
	return &AtomicCounter{}
}

// Increment increments the counter
func (ac *AtomicCounter) Increment() {
	ac.value.Add(1)
}

// Get returns the current value
func (ac *AtomicCounter) Get() int64 {
	return ac.value.Load()
}

// WaitForValue waits for the counter to reach a specific value
// ctx: parent context from command entry point (should not be created here)
func (ac *AtomicCounter) WaitForValue(ctx context.Context, value int64, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return WaitForCondition(
		timeoutCtx,
		func() bool { return ac.value.Load() >= value },
		10*time.Millisecond,
	)
}

// EventCollector collects events and provides deterministic waiting
type EventCollector struct {
	mu     sync.Mutex
	events []any
	notify chan struct{}
}

// NewEventCollector creates a new event collector
func NewEventCollector() *EventCollector {
	return &EventCollector{
		events: make([]any, 0),
		notify: make(chan struct{}, 1),
	}
}

// Add adds an event to the collector
func (ec *EventCollector) Add(event any) {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.events = append(ec.events, event)
	select {
	case ec.notify <- struct{}{}:
	default:
	}
}

// GetEvents returns all collected events
func (ec *EventCollector) GetEvents() []any {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	result := make([]any, len(ec.events))
	copy(result, ec.events)
	return result
}

// WaitForEvents waits for at least N events to be collected
// ctx: parent context from command entry point (should not be created here)
func (ec *EventCollector) WaitForEvents(ctx context.Context, count int, timeout time.Duration) bool {
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
			// Event added, check again
		case <-timeoutCtx.Done():
			return false
		}
	}
}

// WorkerStateTracker tracks worker state changes deterministically
type WorkerStateTracker struct {
	mu     sync.RWMutex
	states map[string]bool // worker ID -> is running
	notify chan string
}

// NewWorkerStateTracker creates a new worker state tracker
func NewWorkerStateTracker() *WorkerStateTracker {
	return &WorkerStateTracker{
		states: make(map[string]bool),
		notify: make(chan string, 10),
	}
}

// SetState sets the state of a worker
func (wst *WorkerStateTracker) SetState(workerID string, running bool) {
	wst.mu.Lock()
	defer wst.mu.Unlock()
	wst.states[workerID] = running
	select {
	case wst.notify <- workerID:
	default:
	}
}

// IsRunning checks if a worker is running
func (wst *WorkerStateTracker) IsRunning(workerID string) bool {
	wst.mu.RLock()
	defer wst.mu.RUnlock()
	return wst.states[workerID]
}

// WaitForRunning waits for a worker to start running
// ctx: parent context from command entry point (should not be created here)
func (wst *WorkerStateTracker) WaitForRunning(ctx context.Context, workerID string, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return WaitForCondition(
		timeoutCtx,
		func() bool { return wst.IsRunning(workerID) },
		10*time.Millisecond,
	)
}

// WaitForStopped waits for a worker to stop
// ctx: parent context from command entry point (should not be created here)
func (wst *WorkerStateTracker) WaitForStopped(ctx context.Context, workerID string, timeout time.Duration) bool {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return WaitForCondition(
		timeoutCtx,
		func() bool { return !wst.IsRunning(workerID) },
		10*time.Millisecond,
	)
}
