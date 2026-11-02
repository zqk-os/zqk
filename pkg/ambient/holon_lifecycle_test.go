package ambient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Traceability: BLI-1793578872469959000-db3a0ac8, CRIT-1793578805152323000-a6cb2a07
// Static Floor Invariant: registry construction and fail-closed registration rules.
func TestHolonLifecycleRegistry_StaticFloorInvariant(t *testing.T) {
	reg := NewHolonLifecycleRegistry()
	if reg == nil {
		t.Fatal("expected non-nil HolonLifecycleRegistry")
	}
	if got := reg.Count(); got != 0 {
		t.Fatalf("expected empty registry, got count=%d", got)
	}

	if err := reg.Register("", "purpose"); !errors.Is(err, ErrHolonIDRequired) {
		t.Fatalf("expected ErrHolonIDRequired, got %v", err)
	}
	if err := reg.Register("watcher", ""); !errors.Is(err, ErrHolonPurposeRequired) {
		t.Fatalf("expected ErrHolonPurposeRequired, got %v", err)
	}
	if err := reg.Register("watcher", "filesystem holon"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Register("watcher", "duplicate"); !errors.Is(err, ErrHolonAlreadyRegistered) {
		t.Fatalf("expected ErrHolonAlreadyRegistered, got %v", err)
	}

	rec, ok := reg.Get("watcher")
	if !ok {
		t.Fatal("expected registered holon")
	}
	if rec.Phase != HolonPhaseRegistered {
		t.Fatalf("expected phase %q, got %q", HolonPhaseRegistered, rec.Phase)
	}
	if rec.Purpose != "filesystem holon" {
		t.Fatalf("unexpected purpose %q", rec.Purpose)
	}
}

// Operational Dynamic Proof: lifecycle FSM transitions are fail-closed.
func TestHolonLifecycleRegistry_TransitionFailClosed(t *testing.T) {
	reg := NewHolonLifecycleRegistry()
	if err := reg.Register("ingest", "kernel ingest holon"); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := reg.Transition("ingest", HolonPhaseRunning); !errors.Is(err, ErrHolonIllegalTransition) {
		t.Fatalf("registered→running must fail-closed, got %v", err)
	}
	if err := reg.Transition("missing", HolonPhaseStarting); !errors.Is(err, ErrHolonNotFound) {
		t.Fatalf("expected ErrHolonNotFound, got %v", err)
	}

	mustTransition(t, reg, "ingest", HolonPhaseStarting)
	mustTransition(t, reg, "ingest", HolonPhaseRunning)
	mustTransition(t, reg, "ingest", HolonPhaseStopping)
	mustTransition(t, reg, "ingest", HolonPhaseStopped)

	if err := reg.Transition("ingest", HolonPhaseRunning); !errors.Is(err, ErrHolonIllegalTransition) {
		t.Fatalf("stopped→running must fail-closed, got %v", err)
	}
	mustTransition(t, reg, "ingest", HolonPhaseStarting) // restart path
	mustTransition(t, reg, "ingest", HolonPhaseFailed)
	mustTransition(t, reg, "ingest", HolonPhaseRegistered) // recovery re-arm
}

// Operational Dynamic Proof: concurrency supervisor tracks managed goroutines and
// AssertZeroUnmanaged fails closed while work is outstanding.
func TestInvariantConcurrencySupervisor_ZeroUnmanaged(t *testing.T) {
	defer goleak.VerifyNone(t)

	reg := NewHolonLifecycleRegistry()
	if err := reg.Register("hub", "event hub holon"); err != nil {
		t.Fatalf("register: %v", err)
	}
	mustTransition(t, reg, "hub", HolonPhaseStarting)
	mustTransition(t, reg, "hub", HolonPhaseRunning)

	budget := goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 4})
	sup := NewInvariantConcurrencySupervisor(reg, budget)

	if err := sup.AssertZeroUnmanaged(); err != nil {
		t.Fatalf("idle supervisor must report zero unmanaged: %v", err)
	}

	release := make(chan struct{})
	var started atomic.Bool
	if err := sup.Go(context.Background(), "hub", "hub_worker", "process ambient events", func(ctx context.Context) {
		started.Store(true)
		select {
		case <-release:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatalf("Go: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("managed goroutine did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := sup.AssertZeroUnmanaged(); !errors.Is(err, ErrUnmanagedGoroutines) {
		t.Fatalf("expected ErrUnmanagedGoroutines while worker live, got %v", err)
	}
	if got := sup.ActiveCount("hub"); got != 1 {
		t.Fatalf("expected ActiveCount=1, got %d", got)
	}

	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sup.StopHolon(ctx, "hub"); err != nil {
		t.Fatalf("StopHolon: %v", err)
	}
	rec, ok := reg.Get("hub")
	if !ok || rec.Phase != HolonPhaseStopped {
		t.Fatalf("expected stopped holon, got ok=%v phase=%q", ok, rec.Phase)
	}
	if err := sup.AssertZeroUnmanaged(); err != nil {
		t.Fatalf("after StopHolon expected zero unmanaged: %v", err)
	}
}

func TestInvariantConcurrencySupervisor_FailClosedWhenNotRunning(t *testing.T) {
	defer goleak.VerifyNone(t)

	reg := NewHolonLifecycleRegistry()
	if err := reg.Register("hygiene", "branch hygiene holon"); err != nil {
		t.Fatalf("register: %v", err)
	}
	sup := NewInvariantConcurrencySupervisor(reg, goroutinelabels.NewBudget(goroutinelabels.BudgetConfig{MaxTotal: 1}))

	err := sup.Go(context.Background(), "hygiene", "worker", "should fail", func(ctx context.Context) {})
	if !errors.Is(err, ErrHolonNotRunning) {
		t.Fatalf("expected ErrHolonNotRunning, got %v", err)
	}

	mustTransition(t, reg, "hygiene", HolonPhaseStarting)
	mustTransition(t, reg, "hygiene", HolonPhaseRunning)

	hold := make(chan struct{})
	var started atomic.Bool
	if err := sup.Go(context.Background(), "hygiene", "holder", "hold budget slot", func(ctx context.Context) {
		started.Store(true)
		select {
		case <-hold:
		case <-ctx.Done():
		}
	}); err != nil {
		t.Fatalf("Go holder: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("holder did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	err = sup.Go(context.Background(), "hygiene", "overflow", "over budget", func(ctx context.Context) {})
	if err == nil {
		t.Fatal("expected budget exceed error")
	}

	close(hold)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sup.StopHolon(ctx, "hygiene"); err != nil {
		t.Fatalf("StopHolon: %v", err)
	}
}

func mustTransition(t *testing.T, reg *HolonLifecycleRegistry, id string, to HolonPhase) {
	t.Helper()
	if err := reg.Transition(id, to); err != nil {
		t.Fatalf("transition %s→%s: %v", id, to, err)
	}
}
