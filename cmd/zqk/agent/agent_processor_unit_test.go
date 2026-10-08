package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNewAgentProcessor_ValidationBranches(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()
	t.Setenv("ZQK_PROJECT_ROOT", tempDir)

	cmd := NewOrchestrateCmd()
	cmd.SetContext(pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext()))
	proc, err := newAgentProcessor(cmd)
	require.NoError(t, err)
	assert.NotNil(t, proc)
	assert.Equal(t, tempDir, proc.ProjectRoot())
}

func TestNewAgentCmd(t *testing.T) {
	cmd := NewAgentCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent", cmd.Use)
	assert.NotEmpty(t, cmd.Commands())
}

func TestProcStorageTuple(t *testing.T) {
	ctx, secCtx, sp := procStorageTuple(nil)
	assert.NotNil(t, ctx)
	assert.NotNil(t, secCtx)
	assert.Nil(t, sp)
}

func TestSeatWorker_MarksAndEnv(t *testing.T) {
	root := t.TempDir()

	// writePlanOrchSubmitMark & recentPlanOrchSubmit
	require.NoError(t, writePlanOrchSubmitMark(root, "PRI-TEST-001", "some detail"))
	recent, msg := recentPlanOrchSubmit(root, "PRI-TEST-001")
	assert.True(t, recent)
	assert.Contains(t, msg, "already_submitted")

	// non-existent plan
	recent2, _ := recentPlanOrchSubmit(root, "PRI-NONEXISTENT")
	assert.False(t, recent2)

	// writeFillSubmitMark & recentFillSubmit
	require.NoError(t, writeFillSubmitMark(root, "goal", "goal detail"))
	recentFill, fillMsg := recentFillSubmit(root, "goal")
	assert.True(t, recentFill)
	assert.Contains(t, fillMsg, "already_submitted")

	recentFill2, _ := recentFillSubmit(root, "other_kind")
	assert.False(t, recentFill2)

	// withLocalLLMEnv & firstNonEmptyEnv
	env := withLocalLLMEnv([]string{"EXISTING=1"})
	assert.NotEmpty(t, env)
	assert.Equal(t, "val", firstNonEmptyEnv("", "  ", "val", "other"))
	assert.Equal(t, "", firstNonEmptyEnv("", "  "))
}

func TestHandleNonCommsWithAgentX_ActivePlanAndDocsEval(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	logger := logging.GetLoggerFromProfile("")

	// 1. Active plan directive
	activePlan := map[string]any{
		objects.FieldKeyID:            "PRI-ACTIVE-DIRECTIVE-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Active Directive Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyDescription:   "Active execution plan",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, activePlan))

	item := agentfeed.CorrespondenceItem{EventID: "EVT-ACTIVE-DIR-001"}
	err := handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", "PER-COMMUNITY-TPM", "SES-1", item, "ORCHESTRATE_PLAN PRI-ACTIVE-DIRECTIVE-001")
	assert.NoError(t, err)

	// 2. DocsEval task
	docsTask := map[string]any{
		objects.FieldKeyID:                 "ATK-DOCSEVAL-001",
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Docs Evaluation for Architecture",
		objects.FieldKeyStatus:             objects.ObjectStatusApproved,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		objects.FieldKeyDescription:        "Evaluate docs and architecture",
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, docsTask))

	itemDocs := agentfeed.CorrespondenceItem{EventID: "EVT-DOCS-001"}
	errDocs := handleNonCommsWithAgentX(ctx, logger, provider, secCtx, tempDir, "SEAT-1", objects.ConstPersonaDefaultOperator, "SES-1", itemDocs, "Execute ATK-DOCSEVAL-001")
	assert.NoError(t, errDocs)
}
