package pm

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/dna"
)

func TestBacklogItem_GetID(t *testing.T) {
	t.Parallel()

	// Direct ID
	b1 := &BacklogItem{ID: "BLI-101"}
	if b1.GetID() != "BLI-101" {
		t.Errorf("expected BLI-101, got %s", b1.GetID())
	}

	// From URN
	u, _ := dna.NewURN("pm", "backlog_item", "BLI-102")
	b2 := &BacklogItem{
		BaseObject: dna.BaseObject{URN: u},
	}
	if b2.GetID() != "BLI-102" {
		t.Errorf("expected BLI-102, got %s", b2.GetID())
	}
}

func TestNewBacklogItem_ValidationErrors(t *testing.T) {
	t.Parallel()

	if _, err := NewBacklogItem("", "title", "desc"); err == nil {
		t.Errorf("expected error on empty id")
	}
	if _, err := NewBacklogItem("id", "", "desc"); err == nil {
		t.Errorf("expected error on empty title")
	}
	if _, err := NewBacklogItem("id", "title", ""); err == nil {
		t.Errorf("expected error on empty desc")
	}
}

func TestInvariantGates_FullCoverage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// NewInvariantGate validation errors
	if _, err := NewInvariantGate("", "task", "done", nil); err == nil {
		t.Errorf("expected error on empty gate ID")
	}
	if _, err := NewInvariantGate("g1", "", "done", nil); err == nil {
		t.Errorf("expected error on empty targetKind")
	}
	if _, err := NewInvariantGate("g1", "task", "", nil); err == nil {
		t.Errorf("expected error on empty targetStatus")
	}

	// Evaluate with nil predicate
	gNil := &InvariantGate{}
	if err := gNil.Evaluate(ctx, nil); err != nil {
		t.Errorf("unexpected error on nil predicate: %v", err)
	}

	// NewProvenanceInvariantGate verification
	pGate, err := NewProvenanceInvariantGate("p-gate", "backlog_item", "complete")
	if err != nil {
		t.Fatalf("NewProvenanceInvariantGate: %v", err)
	}

	// Evaluate on non-provGetter
	if err := pGate.Evaluate(ctx, "string"); err == nil {
		t.Errorf("expected error on non-provGetter")
	}

	// Missing fields
	type mockProv struct {
		prov dna.Provenance
	}
	// Missing hash
	m := &mockAuditable{prov: dna.Provenance{}}
	if err := pGate.Evaluate(ctx, m); err == nil {
		t.Errorf("expected error on missing hash")
	}
	// Missing signature
	m.prov.Hash = "h1"
	if err := pGate.Evaluate(ctx, m); err == nil {
		t.Errorf("expected error on missing signature")
	}
	// Missing parent hash
	m.prov.Signature = "sig1"
	if err := pGate.Evaluate(ctx, m); err == nil {
		t.Errorf("expected error on missing parent hash")
	}
	// Complete
	m.prov.ParentHash = "parent1"
	if err := pGate.Evaluate(ctx, m); err != nil {
		t.Errorf("unexpected error on valid provenance: %v", err)
	}
}

type mockAuditable struct {
	prov dna.Provenance
}

func (m *mockAuditable) GetProvenance() dna.Provenance {
	return m.prov
}

func TestEpic_Methods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Validation
	if _, err := NewEpic("", "title", "desc"); err == nil {
		t.Errorf("expected error on empty epic ID")
	}
	if _, err := NewEpic("e1", "", "desc"); err == nil {
		t.Errorf("expected error on empty epic title")
	}

	epic, err := NewEpic("epic-1", "My Epic", "Enclave description")
	if err != nil {
		t.Fatalf("NewEpic: %v", err)
	}

	// AddMemberWorkUnit
	epic.AddMemberWorkUnit(nil) // safe no-op
	bli, _ := NewBacklogItem("BLI-1", "Task 1", "Desc 1")
	epic.AddMemberWorkUnit(bli)
	// Duplicate add
	epic.AddMemberWorkUnit(bli)
	if len(epic.MemberWorkUnits) != 2 || len(epic.BacklogItemRefs) != 1 {
		t.Errorf("unexpected member work units / refs: %d / %d", len(epic.MemberWorkUnits), len(epic.BacklogItemRefs))
	}

	// CheckEnclaveScope
	// Empty scope
	epic.EnclaveScope = ""
	if !epic.CheckEnclaveScope("any/target") {
		t.Errorf("empty scope should be unbounded")
	}
	epic.EnclaveScope = "pkg/pm, pkg/dna"
	if !epic.CheckEnclaveScope("pkg/pm/pm.go") {
		t.Errorf("expected match for pkg/pm")
	}
	if epic.CheckEnclaveScope("pkg/other/other.go") {
		t.Errorf("expected no match for pkg/other")
	}

	// CanPromote checks
	// Referenced but not loaded
	epic2, _ := NewEpic("epic-2", "Epic 2", "desc")
	epic2.BacklogItemRefs = []string{"BLI-999"}
	if err := epic2.CanPromote(ctx); err == nil {
		t.Errorf("expected error on referenced but not loaded")
	}

	// Member not complete
	epic.MemberWorkUnits = []*BacklogItem{bli}
	bli.Status = "in_progress"
	if err := epic.CanPromote(ctx); err == nil {
		t.Errorf("expected error on member not complete")
	}

	// Member complete but not PlanePromoted
	bli.Status = "complete"
	bli.Plane = dna.PlaneDraft
	if err := epic.CanPromote(ctx); err == nil {
		t.Errorf("expected error on member not PlanePromoted")
	}

	// Member complete and PlanePromoted
	bli.Plane = dna.PlanePromoted
	if err := epic.CanPromote(ctx); err != nil {
		t.Errorf("unexpected error on valid promotion: %v", err)
	}
}

func TestADR_Methods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	if _, err := NewADR("", "title", "ctx", "dec", "cons"); err == nil {
		t.Errorf("expected error on empty ADR ID")
	}
	if _, err := NewADR("adr-1", "", "ctx", "dec", "cons"); err == nil {
		t.Errorf("expected error on empty ADR title")
	}

	adr, err := NewADR("adr-1", "ADR 1", "Context", "Decision", "Consequences")
	if err != nil {
		t.Fatalf("NewADR: %v", err)
	}

	// Superseded transition invariant gate
	// Step through states to reach accepted
	_ = adr.Transition(ctx, adr, "originated")
	_ = adr.Transition(ctx, adr, "proposed")
	_ = adr.Transition(ctx, adr, "accepted")

	// Attempt superseded without SupersededByRef
	if err := adr.Transition(ctx, adr, "superseded"); err == nil {
		t.Errorf("expected error transitioning to superseded without ref")
	}

	adr.SupersededByRef = "adr-2"
	if err := adr.Transition(ctx, adr, "superseded"); err != nil {
		t.Errorf("unexpected error transitioning to superseded with ref: %v", err)
	}
}
