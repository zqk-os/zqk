package ambient

import (
	"context"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Traceability: BLI-1793578872469959000-db3a0ac8, CRIT-1793578805152323000-a6cb2a07
// Ambient Holon Lifecycle Registry & Invariant Concurrency Supervisor — middle-brain
// units register, transition under a fail-closed FSM, and spawn only managed goroutines.

var (
	// ErrHolonIDRequired is returned when Register is called with an empty id.
	ErrHolonIDRequired = errfmt.Errorf("ambient holon id is required")
	// ErrHolonPurposeRequired is returned when Register is called with an empty purpose.
	ErrHolonPurposeRequired = errfmt.Errorf("ambient holon purpose is required")
	// ErrHolonAlreadyRegistered is returned when a holon id is already present.
	ErrHolonAlreadyRegistered = errfmt.Errorf("ambient holon already registered")
	// ErrHolonNotFound is returned when a transition or lookup targets an unknown id.
	ErrHolonNotFound = errfmt.Errorf("ambient holon not found")
	// ErrHolonIllegalTransition is returned when a lifecycle transition violates the FSM.
	ErrHolonIllegalTransition = errfmt.Errorf("ambient holon illegal lifecycle transition")
	// ErrHolonNotRunning is returned when concurrency work is requested off the running phase.
	ErrHolonNotRunning = errfmt.Errorf("ambient holon is not running")
	// ErrUnmanagedGoroutines is returned when active managed workers remain (invariant breach).
	ErrUnmanagedGoroutines = errfmt.Errorf("ambient unmanaged goroutines present")
	// ErrConcurrencySupervisorMisconfigured is returned when registry or budget is missing.
	ErrConcurrencySupervisorMisconfigured = errfmt.Errorf("ambient concurrency supervisor misconfigured")
)

// HolonPhase is the lifecycle phase of an ambient middle-brain holon.
type HolonPhase string

const (
	// HolonPhaseRegistered is the initial post-registration phase.
	HolonPhaseRegistered HolonPhase = "registered"
	// HolonPhaseStarting is the bootstrapping phase before workers may spawn.
	HolonPhaseStarting HolonPhase = "starting"
	// HolonPhaseRunning allows managed goroutine spawn via the concurrency supervisor.
	HolonPhaseRunning HolonPhase = "running"
	// HolonPhaseStopping drains managed workers before terminal stop.
	HolonPhaseStopping HolonPhase = "stopping"
	// HolonPhaseStopped is the clean terminal phase after drain.
	HolonPhaseStopped HolonPhase = "stopped"
	// HolonPhaseFailed is a fail-closed terminal/recovery phase.
	HolonPhaseFailed HolonPhase = "failed"
)

// HolonRecord is a snapshot of a registered ambient holon.
type HolonRecord struct {
	ID      string
	Purpose string
	Phase   HolonPhase
}

type holonEntry struct {
	id      string
	purpose string
	phase   HolonPhase
}

// HolonLifecycleRegistry enforces fail-closed ambient holon lifecycle transitions.
type HolonLifecycleRegistry struct {
	mu    sync.RWMutex
	holon map[string]*holonEntry
}

// NewHolonLifecycleRegistry constructs an empty ambient holon registry.
func NewHolonLifecycleRegistry() *HolonLifecycleRegistry {
	return &HolonLifecycleRegistry{
		holon: make(map[string]*holonEntry),
	}
}

// Register adds a holon in HolonPhaseRegistered. Empty id/purpose and duplicates fail closed.
func (r *HolonLifecycleRegistry) Register(id, purpose string) error {
	if id == "" {
		return ErrHolonIDRequired
	}
	if purpose == "" {
		return ErrHolonPurposeRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.holon[id]; exists {
		return ErrHolonAlreadyRegistered
	}
	r.holon[id] = &holonEntry{id: id, purpose: purpose, phase: HolonPhaseRegistered}
	return nil
}

// Get returns a copy of the holon record when present.
func (r *HolonLifecycleRegistry) Get(id string) (HolonRecord, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.holon[id]
	if !ok {
		return HolonRecord{}, false
	}
	return HolonRecord{ID: e.id, Purpose: e.purpose, Phase: e.phase}, true
}

// Count returns the number of registered holons.
func (r *HolonLifecycleRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.holon)
}

// Transition applies a fail-closed FSM edge for the named holon.
func (r *HolonLifecycleRegistry) Transition(id string, to HolonPhase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.holon[id]
	if !ok {
		return ErrHolonNotFound
	}
	if !holonTransitionAllowed(e.phase, to) {
		return errfmt.Errorf("%w: %s → %s", ErrHolonIllegalTransition, e.phase, to)
	}
	e.phase = to
	return nil
}

func holonTransitionAllowed(from, to HolonPhase) bool {
	if from == to {
		return false
	}
	switch from {
	case HolonPhaseRegistered:
		return to == HolonPhaseStarting || to == HolonPhaseFailed
	case HolonPhaseStarting:
		return to == HolonPhaseRunning || to == HolonPhaseFailed
	case HolonPhaseRunning:
		return to == HolonPhaseStopping || to == HolonPhaseFailed
	case HolonPhaseStopping:
		return to == HolonPhaseStopped || to == HolonPhaseFailed
	case HolonPhaseStopped:
		return to == HolonPhaseStarting || to == HolonPhaseFailed
	case HolonPhaseFailed:
		return to == HolonPhaseRegistered || to == HolonPhaseStarting
	default:
		return false
	}
}

type holonRuntime struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// InvariantConcurrencySupervisor binds holon lifecycle to a goroutine budget so ambient
// workers are always managed: spawn only while running, drain on stop, assert zero leftover.
type InvariantConcurrencySupervisor struct {
	reg    *HolonLifecycleRegistry
	budget *goroutinelabels.Budget

	mu     sync.Mutex
	active map[string]int
	total  int
	rt     map[string]*holonRuntime
}

// NewInvariantConcurrencySupervisor wires a registry to a goroutine budget.
func NewInvariantConcurrencySupervisor(reg *HolonLifecycleRegistry, budget *goroutinelabels.Budget) *InvariantConcurrencySupervisor {
	return &InvariantConcurrencySupervisor{
		reg:    reg,
		budget: budget,
		active: make(map[string]int),
		rt:     make(map[string]*holonRuntime),
	}
}

// ActiveCount returns the number of live managed goroutines for a holon id.
func (s *InvariantConcurrencySupervisor) ActiveCount(holonID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[holonID]
}

// AssertZeroUnmanaged fails closed when any managed worker is still active.
func (s *InvariantConcurrencySupervisor) AssertZeroUnmanaged() error {
	if s == nil || s.reg == nil || s.budget == nil {
		return ErrConcurrencySupervisorMisconfigured
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.total > 0 {
		return errfmt.Errorf("%w: active=%d", ErrUnmanagedGoroutines, s.total)
	}
	return nil
}

// Go starts a budget-backed labeled goroutine for a running holon. Fail-closed otherwise.
// parent is reserved for API symmetry with ambient Start(ctx); workers are canceled by StopHolon.
func (s *InvariantConcurrencySupervisor) Go(parent context.Context, holonID, name, purpose string, fn func(context.Context)) error {
	if s == nil || s.reg == nil || s.budget == nil {
		return ErrConcurrencySupervisorMisconfigured
	}
	if holonID == "" || name == "" || purpose == "" || fn == nil {
		return errfmt.Errorf("%w: holon_id/name/purpose/fn required", ErrConcurrencySupervisorMisconfigured)
	}
	if parent != nil {
		select {
		case <-parent.Done():
			return parent.Err()
		default:
		}
	}
	rec, ok := s.reg.Get(holonID)
	if !ok {
		return ErrHolonNotFound
	}
	if rec.Phase != HolonPhaseRunning {
		return ErrHolonNotRunning
	}

	release, err := s.budget.Reserve(1)
	if err != nil {
		return err
	}

	s.mu.Lock()
	rt := s.rt[holonID]
	if rt == nil {
		ctx, cancel := context.WithCancel(context.Background())
		rt = &holonRuntime{ctx: ctx, cancel: cancel}
		s.rt[holonID] = rt
	}
	runCtx := rt.ctx
	rt.wg.Add(1)
	s.active[holonID]++
	s.total++
	s.mu.Unlock()

	goroutinelabels.NewGoroutine(name, purpose).
		WithLifecycleContext(runCtx).
		WithCleanup(func() {
			release()
			s.mu.Lock()
			s.active[holonID]--
			if s.active[holonID] < 0 {
				s.active[holonID] = 0
			}
			s.total--
			if s.total < 0 {
				s.total = 0
			}
			s.mu.Unlock()
			rt.wg.Done()
		}).
		StartSimple(func() {
			fn(runCtx)
		})
	return nil
}

// StopHolon cancels managed workers, waits for drain, and transitions running→stopping→stopped.
func (s *InvariantConcurrencySupervisor) StopHolon(ctx context.Context, holonID string) error {
	if s == nil || s.reg == nil || s.budget == nil {
		return ErrConcurrencySupervisorMisconfigured
	}
	rec, ok := s.reg.Get(holonID)
	if !ok {
		return ErrHolonNotFound
	}
	switch rec.Phase {
	case HolonPhaseRunning:
		if err := s.reg.Transition(holonID, HolonPhaseStopping); err != nil {
			return err
		}
	case HolonPhaseStopping:
		// continue drain
	case HolonPhaseStopped:
		return nil
	default:
		return errfmt.Errorf("%w: cannot stop from phase %s", ErrHolonIllegalTransition, rec.Phase)
	}

	s.mu.Lock()
	rt := s.rt[holonID]
	cancel := func() {}
	if rt != nil && rt.cancel != nil {
		cancel = rt.cancel
	}
	s.mu.Unlock()
	cancel()

	if rt != nil {
		done := make(chan struct{})
		goroutinelabels.NewGoroutine("ambient_holon_stop_wait", "wait for managed holon workers to drain").
			AsControlPlane().
			StartSimple(func() {
				rt.wg.Wait()
				close(done)
			})
		select {
		case <-done:
		case <-ctx.Done():
			_ = s.reg.Transition(holonID, HolonPhaseFailed)
			return errfmt.Errorf("%w: stop wait canceled: %v", ErrUnmanagedGoroutines, ctx.Err())
		}
	}

	if err := s.reg.Transition(holonID, HolonPhaseStopped); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.rt, holonID)
	s.mu.Unlock()
	return nil
}
