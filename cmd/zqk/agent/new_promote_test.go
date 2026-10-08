package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
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

func TestRejectNonManualPromoteHop_SameStatus(t *testing.T) {
	if err := rejectNonManualPromoteHop(objects.KindPersona, "approved", "approved"); err != nil {
		t.Fatalf("same status must return nil: %v", err)
	}
}

func TestRejectNonManualPromoteHop_InvalidKind(t *testing.T) {
	if err := rejectNonManualPromoteHop("nonexistent_kind_xyz", "a", "b"); err == nil {
		t.Fatal("expected error for invalid kind")
	}
}

func TestPromoteStatusOneHop_NonExistent(t *testing.T) {
	_, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	err := promoteStatusOneHop(ctx, secCtx, provider, "PER-NONEXISTENT", "approved")
	if err == nil {
		t.Fatal("expected error for non-existent object")
	}
}

func TestRunAgentNew_Execution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. With default description (< 10 chars)
	out, err := executeAgentCommand(t, tempDir, provider, "new", "TestAgentAlpha")
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully initialized agent: TestAgentAlpha")

	// 2. With explicit description
	out, err = executeAgentCommand(t, tempDir, provider, "new", "TestAgentBeta", "--description", "Custom operational persona with extended description.")
	require.NoError(t, err)
	assert.Contains(t, out, "Successfully initialized agent: TestAgentBeta")
}
