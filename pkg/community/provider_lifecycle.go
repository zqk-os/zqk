package community

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// ProviderLifecycleState represents the current operational stage of an extension.
type ProviderLifecycleState string

const (
	LifecycleActive         ProviderLifecycleState = "active"
	LifecycleDraining       ProviderLifecycleState = "draining"
	LifecycleDecommissioned ProviderLifecycleState = "decommissioned"
)

var (
	ErrProviderDecommissioned = errors.New("provider is decommissioned")
	ErrProviderDraining       = errors.New("provider is currently draining: no new requests accepted")
	ErrDrainTimeout           = errors.New("provider drain timed out before all in-flight executions finished")
)

// ProviderLifecycleManager coordinates zero-downtime hot reloading and graceful draining.
type ProviderLifecycleManager struct {
	mu           sync.RWMutex
	state        ProviderLifecycleState
	drainTimeout time.Duration
	wg           sync.WaitGroup
	activeCount  atomic.Int64
}

// NewProviderLifecycleManager creates a lifecycle manager initialized to Active.
func NewProviderLifecycleManager(drainTimeout ...time.Duration) *ProviderLifecycleManager {
	dt := 10 * time.Second
	if len(drainTimeout) > 0 && drainTimeout[0] > 0 {
		dt = drainTimeout[0]
	}

	return &ProviderLifecycleManager{
		state:        LifecycleActive,
		drainTimeout: dt,
	}
}

// State returns the current lifecycle status.
func (lm *ProviderLifecycleManager) State() ProviderLifecycleState {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.state
}

// ActiveCount returns the number of in-flight executions.
func (lm *ProviderLifecycleManager) ActiveCount() int64 {
	return lm.activeCount.Load()
}

// Execute tracks and executes a unit of work if the provider is in the Active state.
func (lm *ProviderLifecycleManager) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if fn == nil {
		return errors.New("handler function cannot be nil")
	}

	lm.mu.RLock()
	if lm.state == LifecycleDecommissioned {
		lm.mu.RUnlock()
		return ErrProviderDecommissioned
	}
	if lm.state == LifecycleDraining {
		lm.mu.RUnlock()
		return ErrProviderDraining
	}

	lm.wg.Add(1)
	lm.activeCount.Add(1)
	lm.mu.RUnlock()

	defer func() {
		lm.activeCount.Add(-1)
		lm.wg.Done()
	}()

	return fn(ctx)
}

// Drain transitions the provider to Draining and blocks until all in-flight executions finish or timeout.
func (lm *ProviderLifecycleManager) Drain(ctx context.Context) error {
	lm.mu.Lock()
	if lm.state == LifecycleDecommissioned {
		lm.mu.Unlock()
		return nil
	}
	lm.state = LifecycleDraining
	lm.mu.Unlock()

	drainCtx, cancel := context.WithTimeout(ctx, lm.drainTimeout)
	defer cancel()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("provider_lifecycle_drain", "await in-flight executions drain").StartSimple(func() {
		lm.wg.Wait()
		close(done)
	})

	select {
	case <-drainCtx.Done():
		lm.Decommission()
		return ErrDrainTimeout
	case <-done:
		lm.Decommission()
		return nil
	}
}

// Decommission permanently moves the provider into the Decommissioned state.
func (lm *ProviderLifecycleManager) Decommission() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.state = LifecycleDecommissioned
}
