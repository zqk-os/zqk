package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestDispatchLeadPlanFromWhatsNext_NoStorage(t *testing.T) {
	t.Parallel()

	res, err := dispatchLeadPlanFromWhatsNext(context.Background(), nil, nil, "/tmp/test", "agent-1", "persona-1")
	require.NoError(t, err)
	assert.Equal(t, "no_storage", res.Skipped)
}

func TestDispatchLeadPlanFromWhatsNext_NoLeadPlan(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	res, err := dispatchLeadPlanFromWhatsNext(context.Background(), nil, provider, tempDir, "agent-1", "persona-1")
	require.NoError(t, err)
	assert.Equal(t, "no_lead_plan", res.Skipped)
}

func TestHandleNonCommsWithAgentX_NoATK(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-NO-ATK-001",
	}

	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, "steer without task id")
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_OrchestrateDirective(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-ORCH-001",
	}

	body := "ORCHESTRATE_PLAN PRI-NONEXISTENT-999"
	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_MaxAttemptsExceeded(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	eventID := "EVT-MAX-ATTEMPTS"
	taskID := "ATK-ATTEMPT-TEST"

	attemptKey := agentfeed.SeatAttemptKey(eventID, taskID)
	agentfeed.RecordSeatEventAttempt(tempDir, "agent-1", attemptKey)
	agentfeed.RecordSeatEventAttempt(tempDir, "agent-1", attemptKey)
	agentfeed.RecordSeatEventAttempt(tempDir, "agent-1", attemptKey)

	item := agentfeed.CorrespondenceItem{
		EventID: eventID,
	}
	body := "ATTN agent-1 claim " + taskID

	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_ClaimFails(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-CLAIM-FAIL",
	}
	body := "ATTN agent-1 claim ATK-NONEXISTENT-FAIL"

	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_DocsEval_ContextRefusal(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-DOCS-EVAL-001"
	require.NoError(t, provider.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:          taskID,
		objects.FieldKeyKind:        objects.KindAgentTask,
		objects.FieldKeyStatus:      objects.ObjectStatusApproved,
		objects.FieldKeyTitle:       "evaluate documentation for model",
		objects.FieldKeyDescription: "docs eval task description",
	}))

	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-DOCS-REFUSAL",
	}
	body := "ATTN agent-1 evaluate docs ATK-DOCS-EVAL-001"

	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestRunAgentSeatWorker_OnceExecution(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// 1. Missing agent-id returns error
	_, err := executeAgentCommand(t, tempDir, provider, "seat-worker", "--once")
	assert.ErrorContains(t, err, "--agent-id is required")

	// 2. Successful once execution with empty inbox
	out, err := executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "test-seat", "--once")
	require.NoError(t, err)
	assert.Contains(t, out, "test-seat")

	// 3. Successful once execution with execute-non-comms
	out, err = executeAgentCommand(t, tempDir, provider, "seat-worker", "--agent-id", "test-coordinator", "--once", "--execute-non-comms")
	require.NoError(t, err)
	assert.Contains(t, out, "test-coordinator")
}

func TestHandleNonCommsWithAgentX_DocsEval_Flow(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskID := "ATK-DOCS-EVAL-TEST"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Documentation Evaluation Run",
		objects.FieldKeyDescription:        "CEF AGENT_AD docs/quality/cef-runs FORBID: source edits",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, task, objects.ObjectStatusInProgress)

	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-DOCS-EVAL-001",
	}
	body := "ATTN agent-1 claim " + taskID + "\nCEF AGENT_AD docs/quality/cef-runs"

	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestDispatchLeadPlanFromWhatsNext_PersonaAndSeatBranches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	plan := map[string]any{
		objects.FieldKeyID:            "PRI-LEAD-BRANCHES",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Lead Active Priority Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, plan, objects.ObjectStatusInProgress)

	bli := map[string]any{
		objects.FieldKeyID:              "BLI-LEAD-PLANNED",
		objects.FieldKeyKind:            objects.KindBacklogItem,
		objects.FieldKeyTitle:           "Planned Unit of Work",
		objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
		objects.FieldKeyPriorityPlanRef: "PRI-LEAD-BRANCHES",
		objects.FieldKeySchemaVersion:   objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, bli, objects.ObjectStatusPlanned)

	// 1. Plan has no persona: skips with no_plan_persona
	res, err := dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "agent-1", "operator")
	require.NoError(t, err)
	assert.Equal(t, "no_plan_persona", res.Skipped)

	// 2. Update plan to have persona, but no worker seat exists in peer_seats
	plan[objects.FieldKeyPersonaRefs] = []any{objects.ConstPersonaDefaultOperator}
	require.NoError(t, provider.Update(ctx, secCtx, "PRI-LEAD-BRANCHES", plan))

	res, err = dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "agent-1", "operator")
	require.NoError(t, err)
	assert.Equal(t, "no_worker_seat", res.Skipped)

	// 3. Save peer seats declaring worker-operator for ConstPersonaDefaultOperator -> dispatches successfully!
	require.NoError(t, agentfeed.SavePeerSeats(tempDir, agentfeed.PeerSeatsFile{
		SchemaVersion: "1",
		Seats: map[string]agentfeed.PeerSeatRecord{
			"worker-operator": {Wake: agentfeed.WakeMembraneAgentAPI, PersonaRef: objects.ConstPersonaDefaultOperator},
		},
	}))

	res, err = dispatchLeadPlanFromWhatsNext(ctx, nil, provider, tempDir, "agent-1", "operator")
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)
	assert.NotEmpty(t, res.EventID)
}

func TestHandleNonCommsWithAgentX_OrchestrateDirective_Flow(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Invalid directive format returns error
	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", agentfeed.CorrespondenceItem{EventID: "EVT-1"}, "ORCHESTRATE_PLAN INVALID@!")
	assert.Error(t, err)

	// 2. Directive with inactive plan triggers ackSeatWorkerSkip
	planPlanned := map[string]any{
		objects.FieldKeyID:            "PRI-INACTIVE-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Inactive Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusComplete,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, planPlanned, objects.ObjectStatusComplete)

	err = handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", agentfeed.CorrespondenceItem{EventID: "EVT-2"}, "ORCHESTRATE_PLAN PRI-INACTIVE-001")
	require.NoError(t, err)

	// 3. Directive with in_progress plan triggers orchestration successfully
	planActive := map[string]any{
		objects.FieldKeyID:            "PRI-ORCH-ACTIVE-001",
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Active Plan",
		objects.FieldKeyStatus:        objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, planActive, objects.ObjectStatusInProgress)

	err = handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", agentfeed.CorrespondenceItem{EventID: "EVT-3"}, "ORCHESTRATE_PLAN PRI-ORCH-ACTIVE-001")
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_ErrorAndAttemptGates(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Steer body with no ATK refuses cold AgentX
	item := agentfeed.CorrespondenceItem{EventID: "EVT-STEER-NO-ATK"}
	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, "STEER without task id")
	require.NoError(t, err)

	// 2. Attempt count exhausted parks event
	taskID := "ATK-EXHAUST-TEST"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Exhaust Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
	}
	storage.CreateCASVisible(t, provider, ctx, secCtx, task, objects.ObjectStatusInProgress)

	attemptKey := agentfeed.SeatAttemptKey("EVT-PARKED", taskID)
	for i := 0; i < seatWorkerMaxEventAttempts; i++ {
		_, _ = agentfeed.RecordSeatEventAttempt(tempDir, "agent-1", attemptKey)
	}
	err = handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", agentfeed.CorrespondenceItem{EventID: "EVT-PARKED"}, "ATTN agent-1 claim "+taskID)
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_MCPFailure(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	taskID := "ATK-DOCS-EVAL-MCP-FAIL"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Documentation Evaluation Run",
		objects.FieldKeyDescription:        "CEF AGENT_AD docs/quality/cef-runs FORBID: source edits",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      objects.DefaultSchemaVersion,
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaDefaultOperator,
		"task_steps": []any{
			map[string]any{
				objects.FieldKeyTitle:  "step 1",
				objects.FieldKeyStatus: objects.ObjectStatusPending,
			},
		},
		"estimated_effort": "2d",
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, task))

	item := agentfeed.CorrespondenceItem{
		EventID: "EVT-MCP-FAIL-001",
	}
	body := "ATTN agent-1 claim " + taskID + "\nCEF AGENT_AD docs/quality/cef-runs"

	t.Setenv("ZQK_BIN", "/nonexistent/path/to/binary")
	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)
}

func TestHandleNonCommsWithAgentX_PlanOrchestrationBranches(t *testing.T) {
	tempDir, provider, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	planID := "PRI-TEST-ORCH-BRANCHES-001"

	// Create a plan with originated status
	plan := map[string]any{
		objects.FieldKeyID:            planID,
		objects.FieldKeyKind:          objects.KindPriorityPlan,
		objects.FieldKeyTitle:         "Originated Plan",
		objects.FieldKeyDescription:   "Test priority plan description for unit testing",
		objects.FieldKeyStatus:        objects.ObjectStatusOriginated,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	require.NoError(t, provider.Create(pkgctx.WithPromoteOnCreate(ctx), secCtx, plan))

	item := agentfeed.CorrespondenceItem{EventID: "EVT-ORCH-ORIG-001"}
	body := "ORCHESTRATE_PLAN " + planID

	// Should skip because status is originated (not active or in_progress)
	err := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item, body)
	require.NoError(t, err)

	// Now write submit mark so recentPlanOrchSubmit returns already_submitted
	require.NoError(t, writePlanOrchSubmitMark(tempDir, planID, "unit test dispatch"))

	item2 := agentfeed.CorrespondenceItem{EventID: "EVT-ORCH-SKIP-002"}
	err2 := handleNonCommsWithAgentX(ctx, nil, provider, secCtx, tempDir, "agent-1", "operator", "session-1", item2, body)
	require.NoError(t, err2)
}

func TestIsMarkDispatchedRecently_Branches(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	// Nonexistent file
	assert.False(t, isMarkDispatchedRecently(tempDir+"/nonexistent.json", 10*time.Minute))

	// Invalid json file
	invalidPath := tempDir + "/invalid.json"
	require.NoError(t, fileutil.WriteFile(invalidPath, []byte("not-json"), 0644))
	assert.False(t, isMarkDispatchedRecently(invalidPath, 10*time.Minute))

	// Valid mark file
	planID := "PRI-MARK-TEST"
	require.NoError(t, writePlanOrchSubmitMark(tempDir, planID, "test mark"))
	markPath := planOrchSubmitMarkPath(tempDir, planID)
	assert.True(t, isMarkDispatchedRecently(markPath, 10*time.Minute))
	assert.False(t, isMarkDispatchedRecently(markPath, 0))
}

func TestRecentPlanOrchSubmitAndAckSkip(t *testing.T) {
	tempDir, _, cleanup := setupAgentInProcessProject(t)
	defer cleanup()

	planID := "PRI-RECENT-SUBMIT"
	recent, msg := recentPlanOrchSubmit(tempDir, planID)
	assert.False(t, recent)
	assert.Empty(t, msg)

	require.NoError(t, writePlanOrchSubmitMark(tempDir, planID, "detail"))
	recent, msg = recentPlanOrchSubmit(tempDir, planID)
	assert.True(t, recent)
	assert.Contains(t, msg, "already_submitted")

	err := ackSeatWorkerSkip(tempDir, "agent-1", "operator", "session-1", "EVT-1", "no work")
	assert.NoError(t, err)
}
