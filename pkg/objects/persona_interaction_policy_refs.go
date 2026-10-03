package objects

import (
	"strings"
)

// PersonaRoleOperator is persona.role for TPM / community operator seats.
// Those seats compile the lead Gantt, not a persona-bound empty column.
// TRACK: follow-up in kernel backlog
const PersonaRoleOperator = "operator"

// PersonaSeesLeadGantt is true when the seated persona is a steward (operator).
func PersonaSeesLeadGantt(role string) bool {
	return strings.EqualFold(strings.TrimSpace(role), PersonaRoleOperator)
}

// CollectPersonaInteractionPolicyRefs returns POL-* ids bound on a persona via
// canonical interaction_policy_refs and/or related_object_refs.
// TRACK: dual-read until all personas migrate.
func CollectPersonaInteractionPolicyRefs(persona map[string]any) []string {
	return collectPrefixedPersonaRefs(persona, "POL-", FieldKeyInteractionPolicyRefs)
}
