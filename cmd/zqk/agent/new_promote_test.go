package agent

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestRejectNonManualPromoteHop_PersonaProposedToApproved(t *testing.T) {
	if err := rejectNonManualPromoteHop(objects.KindPersona, objects.ObjectStatusProposed, objects.ObjectStatusApproved); err != nil {
		t.Fatalf("proposed → approved must be a manual one-hop: %v", err)
	}
}

func TestRejectNonManualPromoteHop_PersonaProposedToImplemented(t *testing.T) {
	err := rejectNonManualPromoteHop(objects.KindPersona, objects.ObjectStatusProposed, objects.ObjectStatusImplemented)
	if err == nil {
		t.Fatal("proposed → implemented must not pass the promote neighbor check")
	}
}

func TestRejectNonManualPromoteHop_PersonaProposedToArchived(t *testing.T) {
	// Update would allow from: '*' → archived; promote must not.
	err := rejectNonManualPromoteHop(objects.KindPersona, objects.ObjectStatusProposed, objects.ObjectStatusArchived)
	if err == nil {
		t.Fatal("proposed → archived is a wildcard edge, not a progress promote hop")
	}
}

func TestRejectNonManualPromoteHop_SkillProposedToApproved(t *testing.T) {
	if err := rejectNonManualPromoteHop(objects.KindAgentSkill, objects.ObjectStatusProposed, objects.ObjectStatusApproved); err != nil {
		t.Fatalf("agent_skill proposed → approved must be a manual one-hop: %v", err)
	}
}
