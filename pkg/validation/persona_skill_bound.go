package validation

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Canonical criteria id for CRI-PERSONA-SKILL-BOUND (MMORCH).
// TRACK: REDACTED — keep in sync with kernel criteria object.
const CriteriaIDPersonaSkillBound = "REDACTED"

// PersonaSkillBoundResult is the CRI-PERSONA-SKILL-BOUND gate outcome for one persona.
type PersonaSkillBoundResult struct {
	Bound   bool
	ASKRefs []string
	Missing []string
}

// ASKResolver looks up an agent_skill by id. Return nil map if missing.
// When nil, EvaluatePersonaSkillBound only checks that ASK refs are present (structural).
type ASKResolver func(askID string) (obj map[string]any, err error)

// EvaluatePersonaSkillBound checks CRI-PERSONA-SKILL-BOUND against a persona map.
// Dual-reads agent_skill_refs and related_object_refs ASK-* until migration completes.
// TRACK: REDACTED
func EvaluatePersonaSkillBound(persona map[string]any, resolve ASKResolver) PersonaSkillBoundResult {
	if persona == nil {
		return PersonaSkillBoundResult{Bound: false, Missing: []string{"object"}}
	}
	refs := objects.CollectPersonaASKRefs(persona)
	if len(refs) == 0 {
		return PersonaSkillBoundResult{Bound: false, Missing: []string{objects.FieldKeyAgentSkillRefs}}
	}
	if resolve == nil {
		return PersonaSkillBoundResult{Bound: true, ASKRefs: refs}
	}
	var missing []string
	okRefs := make([]string, 0, len(refs))
	for _, id := range refs {
		obj, err := resolve(id)
		if err != nil || obj == nil {
			missing = append(missing, id+":missing")
			continue
		}
		st, _ := obj[objects.FieldKeyStatus].(string)
		st = strings.ToLower(strings.TrimSpace(st))
		if st == objects.ObjectStatusArchived || st == "error" {
			missing = append(missing, id+":"+st)
			continue
		}
		okRefs = append(okRefs, id)
	}
	if len(okRefs) == 0 {
		if len(missing) == 0 {
			missing = []string{objects.FieldKeyAgentSkillRefs}
		}
		return PersonaSkillBoundResult{Bound: false, ASKRefs: refs, Missing: missing}
	}
	return PersonaSkillBoundResult{Bound: true, ASKRefs: okRefs, Missing: missing}
}

// IsPersonaSkillBound is the boolean form of EvaluatePersonaSkillBound.
func IsPersonaSkillBound(persona map[string]any, resolve ASKResolver) bool {
	return EvaluatePersonaSkillBound(persona, resolve).Bound
}
