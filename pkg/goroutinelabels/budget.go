package goroutinelabels

import (
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

var (
	defaultBudgetMu sync.Mutex
	defaultBudget   *Budget
)

// SetDefaultBudget sets the process-wide goroutine budget. Optional; if not set,
// NewPool and WithBudget require an explicit Budget. When set, DefaultBudget() returns it
// so components can create pools and one-off goroutines against the same cap.
func SetDefaultBudget(b *Budget) {
	defaultBudgetMu.Lock()
	defer defaultBudgetMu.Unlock()
	defaultBudget = b
}

// DefaultBudget returns the process-wide budget, or nil if not set.
func DefaultBudget() *Budget {
	defaultBudgetMu.Lock()
	defer defaultBudgetMu.Unlock()
	return defaultBudget
}

// Budget enforces a global goroutine cap by tracking reserved slots.
// Pools reserve N slots at creation and release them on Stop().
// One-off goroutines (via GoroutineBuilder.WithBudget) reserve 1 when started and release on exit.
//
// Use this to bound total goroutines: create a Budget at process start, then create all
// pools and labeled goroutines through it so the total never exceeds MaxTotal.
type Budget struct {
	mu       sync.Mutex
	maxTotal int          // 0 = unbounded (no enforcement)
	reserved atomic.Int64 // sum of all currently reserved slots
}

// BudgetConfig configures a Budget. MaxTotal is the global cap; 0 means unbounded.
type BudgetConfig struct {
	MaxTotal int
}

// NewBudget creates a Budget. MaxTotal 0 means unbounded (Reserve always succeeds).
func NewBudget(cfg BudgetConfig) *Budget {
	return &Budget{maxTotal: cfg.MaxTotal}
}

// Reserve reserves n goroutine slots from the budget. Returns a release function that must
// be called when the goroutines are no longer running (e.g. when a pool is stopped, or
// when a one-off goroutine exits). If n would exceed the available budget, returns an error
// and the caller must not start the goroutines.
//
// When maxTotal is 0 (unbounded), Reserve always succeeds.
func (b *Budget) Reserve(n int) (release func(), err error) {
	if n <= 0 {
		return func() {}, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	curReserved := int(b.reserved.Load())
	if b.maxTotal > 0 && curReserved+n > b.maxTotal {
		return nil, errfmt.Errorf("goroutine budget exceeded: reserved=%d requested=%d maxTotal=%d",
			curReserved, n, b.maxTotal)
	}
	b.reserved.Add(int64(n))
	released := false
	release = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if released {
			return
		}
		released = true
		b.reserved.Add(int64(-n))
		if b.reserved.Load() < 0 {
			b.reserved.Store(0)
		}
	}
	return release, nil
}

// Available returns how many slots are still available (maxTotal - reserved).
// If maxTotal is 0, returns a large sentinel so callers can treat as "unbounded".
func (b *Budget) Available() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxTotal == 0 {
		return 1 << 30 // effectively unbounded
	}
	avail := b.maxTotal - int(b.reserved.Load())
	if avail < 0 {
		return 0
	}
	return avail
}

// Reserved returns the number of slots currently reserved.
func (b *Budget) Reserved() int {
	return int(b.reserved.Load())
}

// MaxTotal returns the configured cap (0 = unbounded).
func (b *Budget) MaxTotal() int {
	return b.maxTotal
}
