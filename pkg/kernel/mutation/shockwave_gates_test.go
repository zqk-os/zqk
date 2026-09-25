package mutation_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/zqk-os/zqk/pkg/kernel/mutation"
)

// TST-KERNEL-SHOCKWAVE-GATES-SUITE: Composite Execution Organizer Test Harness
// Verifies:
// 1. Static Floor: CRIT-SHOCKWAVE-STATIC-LIFECYCLE-GATES
// 2. Operational Proof: CRIT-SHOCKWAVE-OPERATIONAL-RESTRICTION
// 3. Negative Invariant: CRIT-SHOCKWAVE-ADVERSARIAL-MUTATION-REJECTION

// TestShockwaveStaticLifecycleGates asserts that illegal status jumps are blocked.
func TestShockwaveStaticLifecycleGates(t *testing.T) {
	ge := mutation.NewGateEngine()
	ctx := context.Background()

	t.Run("Rejects Illegal Jump from Originated Directly to InProgress", func(t *testing.T) {
		req := mutation.KernelMutationRequest{
			ObjectID:      "BLI-TEST-001",
			Kind:          mutation.KindBacklogItem,
			CurrentStatus: mutation.StatusOriginated,
			TargetStatus:  mutation.StatusInProgress, // Illegal skip of StatusPlanned
			MilestoneRefs: []string{"MIL-001"},
		}

		res := ge.EvaluateMutation(ctx, req)
		if res.Allowed {
			t.Fatal("expected illegal status jump to be rejected, got allowed")
		}
		if !errors.Is(res.Error, mutation.ErrInvalidStatusTransition) {
			t.Errorf("expected ErrInvalidStatusTransition, got %v", res.Error)
		}
	})

	t.Run("Accepts Canonical Legal Sequence", func(t *testing.T) {
		req := mutation.KernelMutationRequest{
			ObjectID:      "BLI-TEST-002",
			Kind:          mutation.KindBacklogItem,
			CurrentStatus: mutation.StatusOriginated,
			TargetStatus:  mutation.StatusPlanned,
			MilestoneRefs: []string{"MIL-001"},
		}

		res := ge.EvaluateMutation(ctx, req)
		if !res.Allowed {
			t.Fatalf("expected legal transition to be allowed, got error: %v", res.Error)
		}
		if res.Shockwave.RollbackNeeded {
			t.Errorf("expected RollbackNeeded == false for legal transition")
		}
	})
}

// TestShockwaveOperationalRestriction asserts shockwave emission and rollback signals.
func TestShockwaveOperationalRestriction(t *testing.T) {
	ge := mutation.NewGateEngine()
	ctx := context.Background()

	var eventsEmitted int32
	var rollbackEvents int32

	ge.Subscribe(func(event mutation.ShockwaveEvent) {
		atomic.AddInt32(&eventsEmitted, 1)
		if event.RollbackNeeded {
			atomic.AddInt32(&rollbackEvents, 1)
		}
	})

	// Fire invalid mutation
	req := mutation.KernelMutationRequest{
		ObjectID:      "PRI-INVALID-HOP",
		Kind:          mutation.KindPriorityPlan,
		CurrentStatus: mutation.StatusActive,
		TargetStatus:  mutation.StatusConceptual, // Illegal backwards jump
	}

	res := ge.EvaluateMutation(ctx, req)
	if res.Allowed {
		t.Fatal("expected illegal priority plan jump to be disallowed")
	}

	if atomic.LoadInt32(&eventsEmitted) != 1 {
		t.Errorf("expected 1 shockwave event emitted, got %d", atomic.LoadInt32(&eventsEmitted))
	}
	if atomic.LoadInt32(&rollbackEvents) != 1 {
		t.Errorf("expected 1 rollback shockwave event, got %d", atomic.LoadInt32(&rollbackEvents))
	}
	if res.Shockwave.Reason == "" {
		t.Errorf("expected non-empty shockwave diagnostic reason")
	}
}

// TestShockwaveAdversarialMutationRejection asserts that objects with broken lineage
// or uncompleted criteria fail-closed when attempting state promotions.
func TestShockwaveAdversarialMutationRejection(t *testing.T) {
	ge := mutation.NewGateEngine()
	ctx := context.Background()

	t.Run("Rejects Promotion of Backlog Item Missing Milestone Reference", func(t *testing.T) {
		req := mutation.KernelMutationRequest{
			ObjectID:      "BLI-ORPHAN",
			Kind:          mutation.KindBacklogItem,
			CurrentStatus: mutation.StatusOriginated,
			TargetStatus:  mutation.StatusPlanned,
			MilestoneRefs: nil, // Missing required milestone reference
		}

		res := ge.EvaluateMutation(ctx, req)
		if res.Allowed {
			t.Fatal("expected promotion without milestone to fail closed")
		}
		if !errors.Is(res.Error, mutation.ErrIncompleteLineage) {
			t.Errorf("expected ErrIncompleteLineage, got %v", res.Error)
		}
	})

	t.Run("Rejects Completion Hop with Unbound or Incomplete Criteria", func(t *testing.T) {
		req := mutation.KernelMutationRequest{
			ObjectID:         "BLI-UNBOUND-CRIT",
			Kind:             mutation.KindBacklogItem,
			CurrentStatus:    mutation.StatusInProgress,
			TargetStatus:     mutation.StatusComplete,
			MilestoneRefs:    []string{"MIL-001"},
			CriteriaComplete: false, // Unbound or failing criteria
		}

		res := ge.EvaluateMutation(ctx, req)
		if res.Allowed {
			t.Fatal("expected completion without verified criteria to fail closed")
		}
		if !errors.Is(res.Error, mutation.ErrUnboundCriteria) {
			t.Errorf("expected ErrUnboundCriteria, got %v", res.Error)
		}
	})
}
