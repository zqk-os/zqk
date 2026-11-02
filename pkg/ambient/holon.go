package ambient

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// HolonState represents the current state of a holon in its lifecycle.
type HolonState string

const (
	HolonStateRegistered HolonState = "registered"
	HolonStateStarting   HolonState = "starting"
	HolonStateRunning    HolonState = "running"
	HolonStateDegraded   HolonState = "degraded"
	HolonStateStopped    HolonState = "stopped"
	HolonStateFailed     HolonState = "failed"
)

// HolonHealth captures runtime health metrics and status for a holon.
type HolonHealth struct {
	State         HolonState `json:"state"`
	Healthy       bool       `json:"healthy"`
	Message       string     `json:"message,omitempty"`
	LastHeartbeat time.Time  `json:"last_heartbeat"`
	RestartCount  int        `json:"restart_count"`
}

// HolonStatus encapsulates identity, ecosystem metadata, and health.
type HolonStatus struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Role      string      `json:"role"`
	Ecosystem string      `json:"ecosystem"`
	Health    HolonHealth `json:"health"`
}

// Holon represents an autonomous middle-brain routine operating within an ecosystem.
type Holon interface {
	ID() string
	Name() string
	Role() string
	Ecosystem() string
	Start(ctx context.Context) error
	Stop() error
	Status() HolonStatus
}

// HolonSupervisor manages the lifecycle of holons, enforcing zero unmanaged goroutines
// and strict structured concurrency invariants.
type HolonSupervisor struct {
	mu        sync.RWMutex
	holons    map[string]Holon
	cancelFns map[string]context.CancelFunc
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	stopped   bool
}

// NewHolonSupervisor initializes a new supervisor with context bounds.
func NewHolonSupervisor(parentCtx context.Context) *HolonSupervisor {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(parentCtx)
	return &HolonSupervisor{
		holons:    make(map[string]Holon),
		cancelFns: make(map[string]context.CancelFunc),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Register adds a holon to the supervisor under registration state.
func (s *HolonSupervisor) Register(h Holon) error {
	if h == nil {
		return errors.New("cannot register nil holon")
	}
	if h.ID() == "" {
		return errors.New("holon ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return errors.New("supervisor has been stopped")
	}
	if _, exists := s.holons[h.ID()]; exists {
		return fmt.Errorf("holon %q is already registered", h.ID())
	}

	s.holons[h.ID()] = h
	return nil
}

func (s *HolonSupervisor) findHolonAndUnlockOnError(id string) (Holon, error) {
	h, exists := s.holons[id]
	if !exists {
		s.mu.Unlock()
		return nil, fmt.Errorf("holon %q not found", id)
	}
	return h, nil
}

// Start launches a registered holon inside a supervised goroutine.
func (s *HolonSupervisor) Start(id string) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return errors.New("supervisor has been stopped")
	}
	h, err := s.findHolonAndUnlockOnError(id)
	if err != nil {
		return err
	}
	if _, running := s.cancelFns[id]; running {
		s.mu.Unlock()
		return fmt.Errorf("holon %q is already running", id)
	}

	holonCtx, holonCancel := context.WithCancel(s.ctx)
	s.cancelFns[id] = holonCancel
	s.wg.Add(1)
	s.mu.Unlock()

	goroutinelabels.NewGoroutine("ambient_holon_runner", "supervised execution of holon").StartSimple(func() {
		s.runHolon(holonCtx, h)
	})
	return nil
}

// runHolon executes the holon with panic recovery and structured cleanup.
func (s *HolonSupervisor) runHolon(ctx context.Context, h Holon) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.handleHolonCrash(h.ID(), fmt.Errorf("panic in holon %s: %v", h.ID(), r))
		}
	}()

	if err := h.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
		s.handleHolonCrash(h.ID(), err)
	}
}

// handleHolonCrash cleans up registration state on crash or failure.
func (s *HolonSupervisor) handleHolonCrash(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cancelFns, id)
}

// Stop terminates an individual running holon gracefully.
func (s *HolonSupervisor) Stop(id string) error {
	s.mu.Lock()
	h, err := s.findHolonAndUnlockOnError(id)
	if err != nil {
		return err
	}
	cancelFn, running := s.cancelFns[id]
	if !running {
		s.mu.Unlock()
		return nil // already stopped
	}
	delete(s.cancelFns, id)
	s.mu.Unlock()

	cancelFn()
	return h.Stop()
}

// Shutdown gracefully stops all running holons and waits for goroutines to exit.
func (s *HolonSupervisor) Shutdown(timeout time.Duration) error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	s.cancel()

	var stopErrors []error
	for id, cancelFn := range s.cancelFns {
		cancelFn()
		if h, ok := s.holons[id]; ok {
			if err := h.Stop(); err != nil {
				stopErrors = append(stopErrors, fmt.Errorf("failed to stop holon %s: %w", id, err))
			}
		}
	}
	s.cancelFns = make(map[string]context.CancelFunc)
	s.mu.Unlock()

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("ambient_holon_shutdown_wait", "wait for supervised holons to stop").StartSimple(func() {
		s.wg.Wait()
		close(done)
	})

	select {
	case <-done:
		return errors.Join(stopErrors...)
	case <-time.After(timeout):
		return fmt.Errorf("supervisor shutdown timed out after %v", timeout)
	}
}

// Status returns a snapshot of status across all managed holons.
func (s *HolonSupervisor) Status() []HolonStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make([]HolonStatus, 0, len(s.holons))
	for _, h := range s.holons {
		statuses = append(statuses, h.Status())
	}
	return statuses
}

// Get retrieves an individual holon by ID.
func (s *HolonSupervisor) Get(id string) (Holon, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, exists := s.holons[id]
	return h, exists
}
