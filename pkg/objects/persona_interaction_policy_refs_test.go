package objects

import "testing"

func TestCollectPersonaInteractionPolicyRefs_DualRead(t *testing.T) {
	t.Parallel()
	p := map[string]any{
		FieldKeyInteractionPolicyRefs: []any{"POL-AGENT-ADMIN-MEMBRANE-001", "not-pol"},
		FieldKeyRelatedObjectRefs:     []any{"BLI-x", "POL-AGENT-TPM-GROOM-AHEAD-001", "POL-AGENT-ADMIN-MEMBRANE-001"},
	}
	got := CollectPersonaInteractionPolicyRefs(p)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "POL-AGENT-ADMIN-MEMBRANE-001" || got[1] != "POL-AGENT-TPM-GROOM-AHEAD-001" {
		t.Fatalf("order/ids=%v", got)
	}
}

func TestPersonaSeesLeadGantt(t *testing.T) {
	t.Parallel()
	if !PersonaSeesLeadGantt(PersonaRoleOperator) {
		t.Fatal("operator must see lead Gantt")
	}
	if PersonaSeesLeadGantt("agent") {
		t.Fatal("agent must keep persona-bound column")
	}
}
