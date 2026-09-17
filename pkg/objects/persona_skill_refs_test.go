package objects

import "testing"

func TestCollectPersonaASKRefs_DualRead(t *testing.T) {
	t.Parallel()
	p := map[string]any{
		FieldKeyAgentSkillRefs:    []any{"ASK-canon-1", "not-ask"},
		FieldKeyRelatedObjectRefs: []any{"BLI-x", "ASK-related-1", "ASK-canon-1"},
	}
	got := CollectPersonaASKRefs(p)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if got[0] != "ASK-canon-1" || got[1] != "ASK-related-1" {
		t.Fatalf("order/ids=%v", got)
	}
}

func TestCollectPersonaASKRefs_ScalarString(t *testing.T) {
	t.Parallel()
	p := map[string]any{FieldKeyAgentSkillRefs: "ASK-one, ASK-two"}
	got := CollectPersonaASKRefs(p)
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}
