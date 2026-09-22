package agent

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestOrchestrationMeshSkillIDs_fromItemAndPersona(t *testing.T) {
	t.Parallel()
	item := map[string]any{
		objects.FieldKeySkillRef:       "ASK-ITEM",
		objects.FieldKeyAgentSkillRefs: []string{"ASK-ITEM-DUP", "ASK-SECOND"},
	}
	persona := map[string]any{
		objects.FieldKeyAgentSkillRefs: []string{"ASK-PERSONA"},
	}
	got := orchestrationMeshSkillIDs(item, persona)
	want := []string{"ASK-ITEM", "ASK-ITEM-DUP", "ASK-SECOND", "ASK-PERSONA"}
	if len(got) != len(want) {
		t.Fatalf("ids=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids=%v want %v", got, want)
		}
	}
}

func TestOrchestrationMeshSkillIDs_emptyWithoutStudioDefault(t *testing.T) {
	t.Parallel()
	got := orchestrationMeshSkillIDs(map[string]any{objects.FieldKeyTitle: "docs"}, nil)
	if len(got) != 0 {
		t.Fatalf("expected no mesh skill ids, got %v", got)
	}
}

func TestOrchestrationTaskBoundaryFor_usesRoleNotIDSubstring(t *testing.T) {
	t.Parallel()
	eng := orchestrationTaskBoundaryFor("software_engineer", "", agentprompt.WorkClassCoding)
	if !eng.CodeValidate {
		t.Fatal("role software_engineer must still validate even if caller id looks like tpm")
	}
}
