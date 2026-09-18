package agent

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestHasPersonaMatch_UnassignedSoftMatch(t *testing.T) {
	pid := "PER-DEFAULT-AGENT"
	unassigned := map[string]any{
		objects.FieldKeyID:    "BLI-1",
		objects.FieldKeyTitle: "hotpath",
	}
	if !hasPersonaMatch(unassigned, []string{pid}) {
		t.Fatal("unassigned BLI must remain eligible under --persona-id (soft match)")
	}

	matching := map[string]any{
		objects.FieldKeyPersonaRefs: []string{pid},
	}
	if !hasPersonaMatch(matching, []string{pid}) {
		t.Fatal("explicit matching persona_refs should pass")
	}

	other := map[string]any{
		objects.FieldKeyPersonaRefs: []string{"PER-OTHER"},
	}
	if hasPersonaMatch(other, []string{pid}) {
		t.Fatal("explicit other persona must be excluded")
	}

	if hasAnyPersonaAssignment(unassigned) {
		t.Fatal("unassigned should report no persona assignment")
	}
	if !hasAnyPersonaAssignment(other) {
		t.Fatal("other should report persona assignment")
	}
}
