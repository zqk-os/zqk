package objects

import (
	"strings"
)

// PersonaRoleOperator is persona.role for TPM / community operator seats.
// Those seats compile the lead Gantt, not a persona-bound empty column.
// TRACK: BLI-REDACTED
const PersonaRoleOperator = "operator"

// PersonaSeesLeadGantt is true when the seated persona is a steward (operator).
func PersonaSeesLeadGantt(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), PersonaRoleOperator)
}

// CollectPersonaInteractionPolicyRefs returns POL-* ids bound on a persona via
// canonical interaction_policy_refs and/or related_object_refs.
// TRACK: BLI-REDACTED — dual-read until all personas migrate.
func CollectPersonaInteractionPolicyRefs(persona map[string]any) []string {
	if persona == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	appendPOL := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if !strings.HasPrefix(strings.ToUpper(id), "POL-") {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range stringListField(persona, FieldKeyInteractionPolicyRefs) {
		appendPOL(id)
	}
	for _, id := range stringListField(persona, FieldKeyRelatedObjectRefs) {
		appendPOL(id)
	}
	return out
}
