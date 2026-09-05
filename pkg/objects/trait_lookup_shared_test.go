package objects

import (
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestLookupTraitRegistryIsShared is the regression guard for the per-call construction that
// made trait lookups re-read the traits directory on every call. Pointer identity is the
// invariant that keeps the cost off the hot path; asserting it here means a future edit that
// swaps in NewTraitRegistry fails a test rather than quietly costing 29s per system check.
func TestLookupTraitRegistryIsShared(t *testing.T) {
	first := lookupTraitRegistry()
	if first == nil {
		t.Fatal("lookupTraitRegistry returned nil")
	}
	for range 100 {
		if got := lookupTraitRegistry(); got != first {
			t.Fatalf("registry not shared: got %p, want %p", got, first)
		}
	}
}

// TestKindHasTraitConcurrentReads exercises the shared registry from many goroutines at once,
// which is how the validation sweep reaches it. The registry carries no mutex, so this test
// exists to be run under -race: it fails if initialization ever stops establishing
// happens-before for later readers, or if something on the lookup path starts mutating maps.
func TestKindHasTraitConcurrentReads(t *testing.T) {
	const goroutines = 16
	kinds := []string{"backlog_item", "agent_task", "priority_plan", "nonexistent_kind_xyz"}

	results := make([]map[string]bool, goroutines)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		idx := g
		goroutinelabels.NewGoroutine("objects_test", "concurrent trait lookup").StartSimple(func() {
			defer wg.Done()
			seen := make(map[string]bool, len(kinds))
			for _, kind := range kinds {
				seen[kind] = KindHasNamedTrait(kind, "completable")
			}
			results[idx] = seen
		})
	}

	// Bounded wait: a shared registry that deadlocked on initialization would otherwise hang
	// the package until the test binary's own timeout, hiding which test was at fault.
	done := make(chan bool)
	goroutinelabels.NewGoroutine("objects_test", "wait for concurrent trait lookups").StartSimple(func() {
		wg.Wait()
		done <- true
	})
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent trait lookups did not complete within 30s")
	}

	// Every goroutine must agree: a shared registry that raced or partially initialized
	// would show up as one worker disagreeing about the same kind.
	for _, kind := range kinds {
		want := results[0][kind]
		for idx, seen := range results {
			if seen[kind] != want {
				t.Errorf("goroutine %d disagrees for %q: got %v, want %v", idx, kind, seen[kind], want)
			}
		}
	}
}

// TestKindHasTraitDoesNotRebuildRegistryPerCall is the guard that actually catches the
// original defect. Pointer identity of lookupTraitRegistry proves the accessor shares, but it
// would still pass if KindHasTrait went back to calling NewTraitRegistry itself — so this
// asserts the cost instead of the plumbing.
//
// Per-call construction re-read the whole traits directory and spawned a goroutine plus a 5s
// timer each time, measured at ~3.5ms per call against real trait data. Shared lookups are map
// reads. The threshold sits ~35x below the regressed cost and ~50x above the healthy cost, so
// it distinguishes the two without being sensitive to machine speed or CI load.
func TestKindHasTraitDoesNotRebuildRegistryPerCall(t *testing.T) {
	const (
		calls  = 500
		budget = 500 * time.Millisecond // regressed cost for 500 calls is ~1.75s
	)
	lookupTraitRegistry() // Pay one-time init outside the measurement.

	start := time.Now()
	for range calls {
		KindHasNamedTrait("backlog_item", "completable")
	}
	elapsed := time.Since(start)

	if elapsed > budget {
		t.Errorf("%d trait lookups took %s, over the %s budget: "+
			"KindHasTrait is likely rebuilding the registry (and re-reading trait files) per call",
			calls, elapsed.Round(time.Millisecond), budget)
	}
}

// TestKindHasTraitStableAcrossCalls pins the behavior the shared registry must preserve:
// repeated lookups return the same answer, and the empty-input contract still short-circuits.
func TestKindHasTraitStableAcrossCalls(t *testing.T) {
	if ok, err := KindHasTrait("", "completable"); ok || err != nil {
		t.Errorf("empty kind: got (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := KindHasTrait("backlog_item", ""); ok || err != nil {
		t.Errorf("empty trait: got (%v, %v), want (false, nil)", ok, err)
	}

	first := KindHasNamedTrait("backlog_item", "completable")
	for range 50 {
		if got := KindHasNamedTrait("backlog_item", "completable"); got != first {
			t.Fatalf("unstable result across calls: got %v, want %v", got, first)
		}
	}
}
