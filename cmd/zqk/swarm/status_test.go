package swarm

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPersonaASKRefs_DualRead(t *testing.T) {
	t.Parallel()
	got := personaASKRefs(map[string]any{
		objects.FieldKeyAgentSkillRefs:    "ASK-a",
		objects.FieldKeyRelatedObjectRefs: []any{"ASK-b", "BLI-x"},
	})
	if len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestPersonaASKRefs_Empty(t *testing.T) {
	t.Parallel()
	if n := len(personaASKRefs(map[string]any{})); n != 0 {
		t.Fatalf("want 0 got %d", n)
	}
}
