package goroutinelabels

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

var (
	PoolCreationDeclinedNotifier   func(name, purpose, reason string)
	poolCreationDeclinedNotifierMu sync.RWMutex
)

// SetPoolCreationDeclinedNotifier sets the PoolCreationDeclinedNotifier safely under lock.
func SetPoolCreationDeclinedNotifier(notifier func(name, purpose, reason string)) {
	poolCreationDeclinedNotifierMu.Lock()
	defer poolCreationDeclinedNotifierMu.Unlock()
	PoolCreationDeclinedNotifier = notifier
}

// poolWork is a single unit of work submitted to a Pool.
type poolWork struct {
	ctx context.Context
	fn  func(context.Context) error
}

// Pool is a bounded worker pool that reserves N goroutine slots from a Budget.
// Workers are started with goroutinelabels; when the pool is stopped, the reservation is released.
// This keeps async, non-blocking behavior while enforcing a global goroutine cap.
// When creation from the budget is not possible, NewPool returns a single-worker fallback pool
// and invokes PoolCreationDeclinedNotifier so the issue can be addressed without spawning unbounded goroutines.
type Pool struct {
	name    string
	purpose string
	size    int
	release func() // return reserved slots to budget when Stop() is called; nil for fallback pools

	work      chan poolWork
	runCtx    context.Context
	runCancel context.CancelFunc
	wg        sync.WaitGroup

	mu         sync.RWMutex
	submitMu   sync.RWMutex
	started    bool
	stopped    bool
	isFallback atomic.Bool // true when this pool was created because budget reserve failed or no budget
}

// NewPool creates a bounded worker pool. If b is nil, or Reserve(workerCount) would fail,
// a single-worker fallback pool is returned and PoolCreationDeclinedNotifier is invoked—
// no error is returned and no unbounded goroutines are spawned.
// queueSize is the capacity of the work channel (0 = unbuffered).
// The pool does not start workers until Start(ctx) is called.
func NewPool(b *Budget, name, purpose string, workerCount, queueSize int) *Pool {
	if b == nil {
		notifyPoolCreationDeclined(name, purpose, "no budget")
		return newFallbackPool(name, purpose, queueSize)
	}
	return b.NewPool(name, purpose, workerCount, queueSize)
}

func notifyPoolCreationDeclined(name, purpose, reason string) {
	poolCreationDeclinedNotifierMu.RLock()
	notifier := PoolCreationDeclinedNotifier
	poolCreationDeclinedNotifierMu.RUnlock()
	if notifier != nil {
		notifier(name, purpose, reason)
	}
}

// newFallbackPool returns a single-worker pool that does not reserve from the budget.
// Used when budget is nil or Reserve fails; keeps behavior bounded and notifies so the issue can be addressed.
func newFallbackPool(name, purpose string, queueSize int) *Pool {
	if queueSize < 0 {
		queueSize = 0
	}
	p := &Pool{
		name:    name,
		purpose: purpose,
		size:    1,
		release: func() {},
		work:    make(chan poolWork, queueSize),
	}
	p.isFallback.Store(true)
	return p
}

// NewPool creates a bounded worker pool that reserves workerCount slots from the budget.
// If workerCount is invalid or Reserve fails, a single-worker fallback pool is returned
// and PoolCreationDeclinedNotifier is invoked; no error is returned.
func (b *Budget) NewPool(name, purpose string, workerCount, queueSize int) *Pool {
	if workerCount <= 0 {
		notifyPoolCreationDeclined(name, purpose, fmt.Sprintf("invalid worker count %d", workerCount))
		return newFallbackPool(name, purpose, queueSize)
	}
	release, err := b.Reserve(workerCount)
	if err != nil {
		notifyPoolCreationDeclined(name, purpose, err.Error())
		return newFallbackPool(name, purpose, queueSize)
	}
	if queueSize < 0 {
		queueSize = 0
	}
	return &Pool{
		name:    name,
		purpose: purpose,
		size:    workerCount,
		release: release,
		work:    make(chan poolWork, queueSize),
	}
}

// IsFallback returns true if this pool is a single-worker fallback created because
// the budget was nil or Reserve failed. Useful for logging or metrics.
func (p *Pool) IsFallback() bool {
	return p.isFallback.Load()
}

// Start starts the worker goroutines. They will run until the context is cancelled or Stop() is called.
// Call Start exactly once; subsequent calls are no-ops.
func (p *Pool) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started || p.stopped {
		p.mu.Unlock()
		return
	}
	p.runCtx, p.runCancel = context.WithCancel(ctx)
	p.started = true
	p.mu.Unlock()

	for i := 0; i < p.size; i++ {
		name := p.name
		purpose := p.purpose
		if p.size > 1 {
			name = fmt.Sprintf("%s_%d", p.name, i)
			purpose = fmt.Sprintf("%s (worker %d)", p.purpose, i)
		}
		NewGoroutine(name, purpose).
			WithWaitGroup(&p.wg).
			StartWithContext(p.runCtx, func(workerCtx context.Context) error {
				for {
					select {
					case w, ok := <-p.work:
						if !ok {
							return nil
						}
						runWorkItemSafely(w)
					case <-workerCtx.Done():
						// Drain remaining buffered work if any before exiting
						for {
							select {
							case w, ok := <-p.work:
								if !ok {
									return nil
								}
								runWorkItemSafely(w)
							default:
								return workerCtx.Err()
							}
						}
					}
				}
			})
	}
}

func runWorkItemSafely(w poolWork) {
	defer func() {
		_ = recover()
	}()
	_ = w.fn(w.ctx) //nolint:errcheck // best-effort per task
}

// Submit enqueues a task to be run by a pool worker. It is non-blocking if the queue has capacity;
// otherwise it blocks until the context is cancelled or space is available.
// If the pool is stopped or the context is cancelled, Submit returns the context error.
func (p *Pool) Submit(ctx context.Context, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()

	p.mu.RLock()
	runCtx := p.runCtx
	stopped := p.stopped
	p.mu.RUnlock()
	if stopped || runCtx == nil {
		return context.Canceled
	}
	w := poolWork{ctx: ctx, fn: fn}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-runCtx.Done():
		return runCtx.Err()
	case p.work <- w:
		return nil
	}
}

// SubmitNonBlocking enqueues a task if the queue has capacity and returns immediately.
// Returns ErrPoolFull if the work channel is full or the pool is stopped.
func (p *Pool) SubmitNonBlocking(ctx context.Context, fn func(context.Context) error) error {
	p.submitMu.RLock()
	defer p.submitMu.RUnlock()

	p.mu.RLock()
	runCtx := p.runCtx
	stopped := p.stopped
	p.mu.RUnlock()
	if stopped || runCtx == nil {
		return context.Canceled
	}
	w := poolWork{ctx: ctx, fn: fn}
	select {
	case <-runCtx.Done():
		return runCtx.Err()
	case p.work <- w:
		return nil
	default:
		return ErrPoolFull
	}
}

// ErrPoolFull is returned when SubmitNonBlocking cannot enqueue because the pool is full.
var ErrPoolFull = errfmt.Errorf("pool work queue full")

// Stop cancels the pool context, closes the work channel, waits for all workers to exit,
// and releases the reserved budget slots. Idempotent; safe to call multiple times.
// If the pool was never started, only the budget reservation is released.
func (p *Pool) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	started := p.started
	cancel := p.runCancel
	p.mu.Unlock()

	if !started {
		if p.release != nil {
			p.release()
			p.release = nil
		}
		return
	}

	// 1. Cancel runCtx first so any pending Submit unblocks.
	if cancel != nil {
		cancel()
	}

	// 2. Lock submitMu to ensure no concurrent Submit call is sending on p.work.
	p.submitMu.Lock()
	close(p.work)
	p.submitMu.Unlock()

	// 3. Wait for workers to drain remaining work and exit.
	p.wg.Wait()

	if p.release != nil {
		p.release()
		p.release = nil
	}
}

// Size returns the number of workers in the pool.
func (p *Pool) Size() int { return p.size }

// Name returns the pool name passed to NewPool (for logs / telemetry).
func (p *Pool) Name() string {
	if p == nil {
		return ""
	}
	return p.name
}

// QueuePressureSnapshot returns worker count and work-queue occupancy (channel length / capacity).
// Approximate under concurrency; intended for diagnostics only (e.g. scheduler dispatch_pressure.jsonl).
func (p *Pool) QueuePressureSnapshot() (workers int, queued int, queueCap int) {
	if p == nil {
		return 0, 0, 0
	}
	p.mu.RLock()
	ch := p.work
	w := p.size
	p.mu.RUnlock()
	if ch != nil {
		return w, len(ch), cap(ch)
	}
	return w, 0, 0
}
