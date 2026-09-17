package pm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lanceman/zqk/pkg/dna"
	"github.com/lanceman/zqk/pkg/errfmt"
)

func TestPMObjectComposition(t *testing.T) {
	ctx := context.Background()

	// 1. BacklogItem Tests
	t.Run("BacklogItem_CompositionAndLifecycle", func(t *testing.T) {
		bli, err := NewBacklogItem("BLI-CELLULAR-TEST-001", "Implement PM Primitives", "Detailed description of cellular PM primitives")
		if err != nil {
			t.Fatalf("unexpected error creating BacklogItem: %v", err)
		}

		expectedURN := "urn:zqk:project-mgmt:backlog_item:BLI-CELLULAR-TEST-001"
		if bli.URN.String() != expectedURN {
			t.Errorf("expected URN %q, got %q", expectedURN, bli.URN.String())
		}
		if bli.Plane != dna.PlaneDraft {
			t.Errorf("expected initial plane %q, got %q", dna.PlaneDraft, bli.Plane)
		}
		if bli.Status != "conceptual" {
			t.Errorf("expected initial status %q, got %q", "conceptual", bli.Status)
		}

		// Provenance Attestation
		bli.Provenance.Attest("ACC-TEST-AGENT", []byte(bli.Title+bli.Description))
		if bli.Provenance.Hash == "" {
			t.Errorf("expected provenance hash to be computed")
		}

		// Register Invariant Gate for transition to in_progress
		bli.RegisterInvariantGate("in_progress", func(ctx context.Context, obj any) error {
			item, ok := obj.(*BacklogItem)
			if !ok {
				return errfmt.Errorf("expected *BacklogItem")
			}
			if len(item.CriteriaRefs) == 0 {
				return errfmt.Errorf("cannot start without linked criteria")
			}
			return nil
		})

		// Step-by-step lifecycle traversal
		if err := bli.Transition(ctx, bli, "originated"); err != nil {
			t.Fatalf("failed transition to originated: %v", err)
		}
		if err := bli.Transition(ctx, bli, "exploring"); err != nil {
			t.Fatalf("failed transition to exploring: %v", err)
		}
		if err := bli.Transition(ctx, bli, "planned"); err != nil {
			t.Fatalf("failed transition to planned: %v", err)
		}
		if err := bli.Transition(ctx, bli, "testing"); err != nil {
			t.Fatalf("failed transition to testing: %v", err)
		}

		// Invariant gate failure (no criteria)
		if err := bli.Transition(ctx, bli, "in_progress"); err == nil {
			t.Errorf("expected invariant gate to block transition to in_progress without criteria")
		}

		// Add criteria and satisfy gate
		bli.CriteriaRefs = append(bli.CriteriaRefs, "CRIT-001")
		if err := bli.Transition(ctx, bli, "in_progress"); err != nil {
			t.Fatalf("expected transition to in_progress to succeed with criteria: %v", err)
		}

		if bli.Status != "in_progress" {
			t.Errorf("expected status in_progress, got %q", bli.Status)
		}
	})

	// 2. InvariantGate Tests
	t.Run("InvariantGate_Evaluation", func(t *testing.T) {
		gate, err := NewInvariantGate("GATE-001", "backlog_item", "complete", func(ctx context.Context, obj any) error {
			if bli, ok := obj.(*BacklogItem); ok {
				if len(bli.CommitHashes) == 0 {
					return errfmt.Errorf("gate failed: missing commit hashes")
				}
				return nil
			}
			return errfmt.Errorf("unsupported object type")
		})
		if err != nil {
			t.Fatalf("unexpected error creating InvariantGate: %v", err)
		}

		expectedURN := "urn:zqk:kernel:invariant_gate:GATE-001"
		if gate.URN.String() != expectedURN {
			t.Errorf("expected URN %q, got %q", expectedURN, gate.URN.String())
		}

		testItem, _ := NewBacklogItem("BLI-TEST-002", "Test Item", "Desc")
		if err := gate.Evaluate(ctx, testItem); err == nil {
			t.Errorf("expected gate evaluation to fail when commit hashes are missing")
		}

		testItem.CommitHashes = append(testItem.CommitHashes, "abc1234")
		if err := gate.Evaluate(ctx, testItem); err != nil {
			t.Errorf("expected gate evaluation to pass when commit hashes are present: %v", err)
		}

		// Test ProvenanceInvariantGate
		provGate, err := NewProvenanceInvariantGate("GATE-PROV-001", "backlog_item", "complete")
		if err != nil {
			t.Fatalf("failed to create ProvenanceInvariantGate: %v", err)
		}
		unattestedItem, _ := NewBacklogItem("BLI-UNATTESTED-001", "Unattested", "Desc")
		if err := provGate.Evaluate(ctx, unattestedItem); err == nil {
			t.Errorf("expected provenance gate to fail on unattested item")
		}

		// Attest item with parent hash and signature
		unattestedItem.Provenance.ParentHash = "genesis_hash_0000"
		unattestedItem.Provenance.Attest("ACC-AGENT-1", []byte("valid payload"))
		if err := provGate.Evaluate(ctx, unattestedItem); err != nil {
			t.Errorf("expected provenance gate to pass on attested item: %v", err)
		}
	})

	// 3. Epic Tests
	t.Run("Epic_Composition_And_Enclave_Scope", func(t *testing.T) {
		epic, err := NewEpic("EPC-001", "Cellular Microkernel Architecture", "High level capability epic")
		if err != nil {
			t.Fatalf("unexpected error creating Epic: %v", err)
		}

		expectedURN := "urn:zqk:project-mgmt:epic:EPC-001"
		if epic.URN.String() != expectedURN {
			t.Errorf("expected URN %q, got %q", expectedURN, epic.URN.String())
		}

		epic.EnclaveScope = "pkg/kernel pkg/dna pkg/pm"
		if !epic.CheckEnclaveScope("pkg/pm/pm.go") {
			t.Errorf("expected pkg/pm/pm.go to be within enclave scope")
		}
		if !epic.CheckEnclaveScope("pkg/kernel/adjacency.go") {
			t.Errorf("expected pkg/kernel/adjacency.go to be within enclave scope")
		}
		if epic.CheckEnclaveScope("external/vendor/other.go") {
			t.Errorf("expected external/vendor/other.go to be outside enclave scope")
		}

		epic.Provenance.Attest("ACC-TEST-LEAD", []byte(epic.Title))
		if epic.Provenance.Hash == "" {
			t.Errorf("expected provenance hash to be computed")
		}

		if err := epic.Transition(ctx, epic, "originated"); err != nil {
			t.Fatalf("failed transition to originated: %v", err)
		}
		if err := epic.Transition(ctx, epic, "active"); err != nil {
			t.Fatalf("failed transition to active: %v", err)
		}
		if epic.Status != "active" {
			t.Errorf("expected status active, got %q", epic.Status)
		}

		// Test Promotion Invariant: Child work units must clear invariant gates and complete
		childBLI, _ := NewBacklogItem("BLI-CHILD-001", "Child Unit", "Must complete first")
		epic.AddMemberWorkUnit(childBLI)

		// Child is still in conceptual status -> Epic transition to complete must fail
		if err := epic.Transition(ctx, epic, "complete"); err == nil {
			t.Errorf("expected epic promotion to fail when member work unit is not complete")
		}

		// Advance child to in_progress then complete
		if err := childBLI.TransitionTo(ctx, "originated"); err != nil {
			t.Fatalf("failed child transition: %v", err)
		}
		if err := childBLI.TransitionTo(ctx, "exploring"); err != nil {
			t.Fatalf("failed child transition: %v", err)
		}
		if err := childBLI.TransitionTo(ctx, "planned"); err != nil {
			t.Fatalf("failed child transition: %v", err)
		}
		if err := childBLI.TransitionTo(ctx, "in_progress"); err != nil {
			t.Fatalf("failed child transition: %v", err)
		}
		if childBLI.Plane != dna.PlaneStaged {
			t.Errorf("expected child plane to be Staged, got %s", childBLI.Plane)
		}

		// Still cannot promote epic while child is in_progress
		if err := epic.Transition(ctx, epic, "complete"); err == nil {
			t.Errorf("expected epic promotion to fail when member work unit is in_progress")
		}

		// Complete child item
		if err := childBLI.TransitionTo(ctx, "complete"); err != nil {
			t.Fatalf("failed child completion: %v", err)
		}
		if childBLI.Plane != dna.PlanePromoted {
			t.Errorf("expected child plane to be Promoted, got %s", childBLI.Plane)
		}

		// Now epic transition to complete must succeed!
		if err := epic.Transition(ctx, epic, "complete"); err != nil {
			t.Fatalf("expected epic promotion to succeed after member work unit complete: %v", err)
		}
		if epic.Status != "complete" {
			t.Errorf("expected epic status complete, got %q", epic.Status)
		}
	})

	// 4. ADR Tests
	t.Run("ADR_CompositionAndLifecycle", func(t *testing.T) {
		adr, err := NewADR(
			"ADR-001",
			"Adopt Cellular Microkernel Primitives",
			"Entities lacked unified DNA and invariant enforcement.",
			"Re-anchor on BaseObject, Auditable, Lifecycle, and MetaSchema.",
			"Eliminates anonymous mutations and state corruption.",
		)
		if err != nil {
			t.Fatalf("unexpected error creating ADR: %v", err)
		}

		expectedURN := "urn:zqk:project-mgmt:adr:ADR-001"
		if adr.URN.String() != expectedURN {
			t.Errorf("expected URN %q, got %q", expectedURN, adr.URN.String())
		}

		if err := adr.Transition(ctx, adr, "originated"); err != nil {
			t.Fatalf("failed transition to originated: %v", err)
		}
		if err := adr.Transition(ctx, adr, "proposed"); err != nil {
			t.Fatalf("failed transition to proposed: %v", err)
		}
		if err := adr.Transition(ctx, adr, "accepted"); err != nil {
			t.Fatalf("failed transition to accepted: %v", err)
		}
		if adr.Status != "accepted" {
			t.Errorf("expected status accepted, got %q", adr.Status)
		}

		// Superseded transition without reference must fail
		if err := adr.Transition(ctx, adr, "superseded"); err == nil {
			t.Errorf("expected transition to superseded to fail without SupersededByRef")
		}

		// Superseded transition with reference must succeed
		adr.SupersededByRef = "urn:zqk:kernel:adr:ADR-002"
		if err := adr.Transition(ctx, adr, "superseded"); err != nil {
			t.Fatalf("failed transition to superseded: %v", err)
		}
		if adr.Status != "superseded" {
			t.Errorf("expected status superseded, got %q", adr.Status)
		}
	})
}

func TestPMObjectJSONSerialization(t *testing.T) {
	bli, err := NewBacklogItem("BLI-JSON-001", "JSON Test", "Ensure serialization works cleanly")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := json.Marshal(bli)
	if err != nil {
		t.Fatalf("failed to marshal BacklogItem to JSON: %v", err)
	}

	var roundtrip BacklogItem
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal BacklogItem: %v", err)
	}

	if roundtrip.URN.String() != bli.URN.String() {
		t.Errorf("URN roundtrip mismatch: got %q, want %q", roundtrip.URN.String(), bli.URN.String())
	}
	if roundtrip.Title != bli.Title {
		t.Errorf("Title roundtrip mismatch: got %q, want %q", roundtrip.Title, bli.Title)
	}
}
