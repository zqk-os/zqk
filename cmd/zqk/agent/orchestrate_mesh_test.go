package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentprompt"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/federation/meshbroker"
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

func TestLeaseOrchestrationMeshSkill(t *testing.T) {
	ctx := context.Background()

	// 1. Nil broker returns empty
	sec, ep := leaseOrchestrationMeshSkill(ctx, nil, nil, nil, nil, "")
	assert.Empty(t, sec)
	assert.Empty(t, ep)

	// 2. Broker with valid storage provider
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	broker := meshbroker.NewSkillBroker(provider, nil, tempDir)
	item := map[string]any{
		objects.FieldKeyTitle: "docs",
	}
	sec, ep = leaseOrchestrationMeshSkill(ctx, provider, nil, broker, item, "")
	assert.Empty(t, sec)
	assert.Empty(t, ep)

	// 3. Broker with existing lease returns leased capability section
	secCtx := pkgctx.NewSystemSecurityContext()
	leaseSession := map[string]any{
		objects.FieldKeyID:                "ZS-LEASE-001",
		objects.FieldKeyKind:              objects.KindZqkSession,
		objects.FieldKeySessionMode:       "federated_lease",
		objects.FieldKeyResourceRef:       "ASK-TEST-SKILL",
		objects.FieldKeyStatus:            objects.ObjectStatusActive,
		objects.FieldKeyProviderKernelRef: "RK-PROVIDER-1",
		objects.FieldKeyTokenID:           "sec-token-123",
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(ctx, secCtx, leaseSession))

	itemWithSkill := map[string]any{
		objects.FieldKeySkillRef: "ASK-TEST-SKILL",
	}
	sec, ep = leaseOrchestrationMeshSkill(ctx, provider, secCtx, broker, itemWithSkill, "")
	assert.Contains(t, sec, "## Leased Mesh Capability")
	assert.Contains(t, sec, "RK-PROVIDER-1")
	assert.Contains(t, sec, "sec-token-123")
	_ = ep
}

func TestItemPersonaID_AllScenarios(t *testing.T) {
	t.Parallel()

	// 1. nil item
	assert.Empty(t, itemPersonaID(nil))

	// 2. empty item
	assert.Empty(t, itemPersonaID(map[string]any{}))

	// 3. assignee_persona_ref
	itemWithAssignee := map[string]any{
		objects.FieldKeyAssigneePersonaRef: "PER-ASSIGNEE-1",
	}
	assert.Equal(t, "PER-ASSIGNEE-1", itemPersonaID(itemWithAssignee))

	// 4. persona_refs as []string
	itemWithSliceString := map[string]any{
		objects.FieldKeyPersonaRefs: []string{"PER-REFS-1", "PER-REFS-2"},
	}
	assert.Equal(t, "PER-REFS-1", itemPersonaID(itemWithSliceString))

	// 5. persona_refs as []any
	itemWithSliceAny := map[string]any{
		objects.FieldKeyPersonaRefs: []any{"PER-ANY-1", "PER-ANY-2"},
	}
	assert.Equal(t, "PER-ANY-1", itemPersonaID(itemWithSliceAny))
}

func TestOrchestrationPersonaRole_Scenarios(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. nil sp returns default
	assert.Equal(t, defaultOrchestrationPersonaRole, orchestrationPersonaRole(ctx, nil, secCtx, "PER-1"))

	// 2. empty personaIDs returns default
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	_ = tempDir

	assert.Equal(t, defaultOrchestrationPersonaRole, orchestrationPersonaRole(ctx, provider, secCtx, "", "  "))

	// 3. non-existent persona returns default
	assert.Equal(t, defaultOrchestrationPersonaRole, orchestrationPersonaRole(ctx, provider, secCtx, "PER-NONEXISTENT"))

	// 4. existing persona returns role
	personaObj := map[string]any{
		objects.FieldKeyID:            "PER-ROLE-TEST",
		objects.FieldKeyKind:          objects.KindPersona,
		objects.FieldKeyTitle:         "Role Test Persona",
		objects.FieldKeyRole:          "system_architect",
		objects.FieldKeyStatus:        objects.ObjectStatusApproved,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(ctx, secCtx, personaObj))

	assert.Equal(t, "system_architect", orchestrationPersonaRole(ctx, provider, secCtx, "PER-ROLE-TEST"))
}
