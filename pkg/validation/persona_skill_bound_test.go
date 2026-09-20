package validation

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestEvaluatePersonaSkillBound_Structural(t *testing.T) {
	t.Parallel()
	unbound := map[string]any{objects.FieldKeyID: "PER-1"}
	if IsPersonaSkillBound(unbound, nil) {
		t.Fatal("expected unbound")
	}
	bound := map[string]any{
		objects.FieldKeyRelatedObjectRefs: []any{"ASK-1"},
	}
	if !IsPersonaSkillBound(bound, nil) {
		t.Fatal("expected bound via related")
	}
}

func TestEvaluatePersonaSkillBound_Resolve(t *testing.T) {
	t.Parallel()
	persona := map[string]any{
		objects.FieldKeyAgentSkillRefs: []any{"ASK-ok", "ASK-arch"},
	}
	resolve := func(id string) (map[string]any, error) {
		switch id {
		case "ASK-ok":
			return map[string]any{objects.FieldKeyStatus: "approved"}, nil
		case "ASK-arch":
			return map[string]any{objects.FieldKeyStatus: objects.ObjectStatusArchived}, nil
		default:
			return nil, nil
		}
	}
	res := EvaluatePersonaSkillBound(persona, resolve)
	if !res.Bound {
		t.Fatalf("expected bound with one live ASK, got %#v", res)
	}
	if len(res.ASKRefs) != 1 || res.ASKRefs[0] != "ASK-ok" {
		t.Fatalf("ASKRefs=%v", res.ASKRefs)
	}

	onlyArch := map[string]any{objects.FieldKeyAgentSkillRefs: []any{"ASK-arch"}}
	if IsPersonaSkillBound(onlyArch, resolve) {
		t.Fatal("archived-only should fail")
	}
}
