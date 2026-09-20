package dna

import (
	"testing"
)

type mockAuditableEntity struct {
	Provenance Provenance
}

func (m *mockAuditableEntity) GetProvenance() Provenance {
	return m.Provenance
}

func TestFoundationalTraits_Auditable(t *testing.T) {
	schema := &MetaSchema{
		TargetKind: "test_entity",
		Traits:     []string{TraitAuditable},
	}

	entity := &mockAuditableEntity{
		Provenance: Provenance{
			AgentID: "",
		},
	}

	// Staged requires AgentID or AgentURN
	err := VetTraitInvariants(schema, entity, MacroPhaseMetabolism, PlaneStaged)
	if err == nil {
		t.Errorf("expected error for empty AgentID in auditable trait, got nil")
	}

	entity.Provenance.AgentID = "agent-42"
	// Promoted requires ParentHash
	err = VetTraitInvariants(schema, entity, MacroPhaseEquilibrium, PlanePromoted)
	if err == nil {
		t.Errorf("expected error for empty ParentHash in auditable trait on PlanePromoted, got nil")
	}

	entity.Provenance.ParentHash = "parent-sha"
	err = VetTraitInvariants(schema, entity, MacroPhaseEquilibrium, PlanePromoted)
	if err != nil {
		t.Fatalf("expected auditable validation to pass, got: %v", err)
	}
}
