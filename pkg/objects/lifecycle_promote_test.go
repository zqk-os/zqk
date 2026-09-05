package objects

import (
	"testing"
)

// TestPromoteTransitionTargets_HopLimit tests that PromoteTransitionTargets
// correctly limits the result to one-hop only. When hopCount > 1 is intended,
// there must be explicit re-probing; this function itself must NOT cascade.
func TestPromoteTransitionTargets_HopLimit(t *testing.T) {
	t.Parallel()
	loader := NewLifecycleLoader("")
	lc, err := loader.LoadLifecycle(KindPriorityPlan)
	if err != nil {
		t.Fatalf("LoadLifecycle(priority_plan): %v", err)
	}

	// From in_progress, PromoteTransitionTargets must return ONLY direct targets.
	targets := PromoteTransitionTargets(lc, ObjectStatusInProgress)
	if len(targets) == 0 {
		t.Fatal("PromoteTransitionTargets(in_progress) returned no targets")
	}

	// Verify: none of the returned targets should have their own PromoteTransitionTargets
	// result include a two-hop-away status.
	for target := range targets {
		nextTargets := PromoteTransitionTargets(lc, target)
		for nt := range nextTargets {
			// If 'nt' is also directly reachable from 'in_progress', that's fine (duplicate direct edge).
			// But if it's NOT directly reachable from in_progress, then the two-hop chain exists.
			if _, ok := targets[nt]; !ok {
				t.Fatalf("overshoot detected: %s→%s is a two-hop target not reachable in one hop; direct targets=%v, nextTargets[%q]=%v",
					ObjectStatusInProgress, target, targets, target, nextTargets)
			}
		}
	}
}

// TestPromoteTransitionTargets_NoCascade tests that PromoteTransitionTargets
// does NOT transitively resolve. Given a lifecycle where:
//
//	in_progress → active
//	active     → complete
//
// Calling PromoteTransitionTargets(from, "in_progress") should only return {"active"},
// never {"complete"} as well.
func TestPromoteTransitionTargets_NoCascade(t *testing.T) {
	t.Parallel()
	loader := NewLifecycleLoader("")
	lc, err := loader.LoadLifecycle(KindPriorityPlan)
	if err != nil {
		t.Fatalf("LoadLifecycle(priority_plan): %v", err)
	}

	targets := PromoteTransitionTargets(lc, ObjectStatusInProgress)

	// in_progress → complete should NOT be a direct target.
	// It should only be reachable via active (2 hops).
	if _, ok := targets[ObjectStatusComplete]; ok {
		t.Fatalf("PROMOTE OVERSHOOT: in_progress→complete is directly reachable but should require 2 hops through active")
	}

	// active should be directly reachable.
	if _, ok := targets[ObjectStatusActive]; !ok {
		t.Fatalf("expected active to be a direct promote target from in_progress; got %#v", targets)
	}
}
