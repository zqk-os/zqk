package workflow

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	pkgsystem "github.com/zqk-os/zqk/pkg/system"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type mockWhatsNextStorage struct {
	storage.ObjectStorageProvider
	data map[string]map[string]any
}

func (m *mockWhatsNextStorage) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.data[id]; ok {
		return obj, nil
	}
	return nil, fmt.Errorf("object %s not found", id)
}

func (m *mockWhatsNextStorage) List(ctx context.Context, sec *pkgctx.SecurityContext, stCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var out []map[string]any
	for _, obj := range m.data {
		kind, _ := obj[objects.FieldKeyKind].(string)
		if filter.Kind != "" && kind != filter.Kind {
			continue
		}
		out = append(out, obj)
	}
	return &storage.QueryResult{Objects: out}, nil
}

func (m *mockWhatsNextStorage) Count(ctx context.Context, sec *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	res, err := m.List(ctx, sec, nil, filter)
	if err != nil {
		return 0, err
	}
	return len(res.Objects), nil
}

func (m *mockWhatsNextStorage) Create(ctx context.Context, sec *pkgctx.SecurityContext, obj map[string]any) error {
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		return fmt.Errorf("missing id")
	}
	m.data[id] = obj
	return nil
}

func (m *mockWhatsNextStorage) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, obj map[string]any) error {
	existing, ok := m.data[id]
	if !ok {
		existing = make(map[string]any)
		m.data[id] = existing
	}
	for k, v := range obj {
		existing[k] = v
	}
	return nil
}

func TestPlanHasWorkAndCountLinkedBLIs(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())

	storeEmpty := newMemoryWorkflowStore()
	assert.False(t, planHasWork(ctx, storeEmpty, "PRI-NONE"))
	assert.Equal(t, 0, countLinkedBLIs(ctx, storeEmpty, "PRI-NONE"))

	storeWithWork := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-1",
			objects.FieldKeyStatus:          objects.ObjectStatusPlanned,
			objects.FieldKeyPriorityPlanRef: "PRI-HAS-WORK",
		},
		map[string]any{
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyID:              "BLI-ARCHIVED",
			objects.FieldKeyStatus:          objects.ObjectStatusArchived,
			objects.FieldKeyPriorityPlanRef: "PRI-HAS-WORK",
		},
	)

	assert.True(t, planHasWork(ctx, storeWithWork, "PRI-HAS-WORK"))
	assert.Equal(t, 1, countLinkedBLIs(ctx, storeWithWork, "PRI-HAS-WORK"))
}

func TestListActiveOrPausedConvergenceSessions(t *testing.T) {
	ctx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())

	store := newMemoryWorkflowStore(
		map[string]any{
			objects.FieldKeyKind:         objects.KindConvergenceSession,
			objects.FieldKeyID:           "CVS-ACTIVE",
			objects.FieldKeyTitle:        "Active Session",
			objects.FieldKeyStatus:       "active",
			objects.FieldKeyCurrentPhase: "measuring",
		},
		map[string]any{
			objects.FieldKeyKind:         objects.KindConvergenceSession,
			objects.FieldKeyID:           "CVS-PAUSED",
			objects.FieldKeyTitle:        "Data cell Paused Session",
			objects.FieldKeyStatus:       "paused",
			objects.FieldKeyCurrentPhase: "idle",
		},
		map[string]any{
			objects.FieldKeyKind:   objects.KindConvergenceSession,
			objects.FieldKeyID:     "CVS-COMPLETE",
			objects.FieldKeyTitle:  "Complete Session",
			objects.FieldKeyStatus: "completed",
		},
	)

	rows := listActiveOrPausedConvergenceSessions(ctx, store)
	require.Len(t, rows, 2)

	// Test pickDefaultMeasureSessionID
	defID := pickDefaultMeasureSessionID(rows)
	// CVS-PAUSED has "Data cell" in title, so it should be prioritized
	assert.Equal(t, "CVS-PAUSED", defID)

	// Rows without data cell
	otherRows := []whatsNextCVSRow{
		{ID: "CVS-1", Title: "Session 1"},
		{ID: "CVS-2", Title: "Session 2"},
	}
	assert.Equal(t, "CVS-1", pickDefaultMeasureSessionID(otherRows))
	assert.Empty(t, pickDefaultMeasureSessionID(nil))
}

func TestCompressWhatsNextMeasure(t *testing.T) {
	raw := map[string]any{
		"unbudgeted_files_count": 5,
		"convergence_ratio":      0.85,
	}
	compressed := compressWhatsNextMeasure(raw)
	assert.NotNil(t, compressed)
}

func TestGetObserverTips(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// No cached tips initially
	tips := getObserverTips()
	assert.Empty(t, tips)
}

func TestWriteWhatsNextTable_Comprehensive(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	cmdStandardCtx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(cmdStandardCtx)

	sessionID := "CVS-MEASURE-01"
	out := whatsNextOut{
		Schema: whatsNextSchema,
		PriorityPlan: &whatsNextPriorityPlan{
			ID:     "PRI-001",
			Title:  "Core Workflow Test",
			Status: "in_progress",
		},
		PackagingCue:                   "Package now: 3 BLIs ready",
		StrategicReplenishmentCue:      "Runway low: stage new plans",
		MaterializedViewRecovering:     true,
		MaterializedViewDegradedReason: "cold cache rebuilding",
		BacklogCountsByStatus: map[string]int{
			"planned":     2,
			"in_progress": 1,
			"completed":   5,
		},
		ConvergenceSessionsActive: []whatsNextCVSRow{
			{ID: "CVS-1", Title: "Convergence 1", Status: "active", CurrentPhase: "phase1"},
		},
		MeasureSessionID:  &sessionID,
		MeasureSkipReason: "",
		MeasureError:      "",
		MeasureCompressed: map[string]any{"ratio": 0.99},
		ObserverTips:      []string{"Tip 1: commit frequently", "Tip 2: verify criteria"},
		KernelAmbience: &whatsnext.KernelAmbience{
			StewardFocus: "Focus on closing Cycle 82",
		},
		RemedyReport: &pkgsystem.RemedyReport{
			TotalApplied: 1,
			Outcomes: []pkgsystem.RemedyOutcome{
				{Applied: true, Plan: pkgsystem.RemedyPlan{Title: "Fix Dir"}, Detail: "Created missing directory"},
			},
		},
		RemedyRecipes: []string{"recipe 1: run vet"},
		FillItem: &whatsnext.FillItem{
			Kind:        "maintenance",
			Reason:      "clean up logs",
			CommandHint: "zqk clean",
		},
		GuidingStep: &whatsNextGuidingStep{
			PolicyID:    "POL-TEST-001",
			GuidingStep: "Run unit tests across package",
		},
		Correspondence: &whatsNextCorrespondence{
			AgentID:        "agent-worker-1",
			PersonaID:      "PER-WORKER",
			NextActionHint: "proceed",
			InboxUnacked: []agentfeed.CorrespondenceItem{
				{EventID: "EVT-1", Summary: "Inbox summary item"},
			},
			RegisteredCallbacks: []agentfeed.PeerAckAwait{
				{ID: "AWAIT-1", EventID: "EVT-2", Action: "ack"},
			},
			ActiveAgentTask: &whatsNextActiveTask{
				ID:     "ATK-1",
				Status: "in_progress",
				Title:  "Implement extended whats-next tests",
			},
		},
	}

	err := writeWhatsNextTable(cmd, out)
	require.NoError(t, err)

	tableStr := buf.String()
	assert.Contains(t, tableStr, "Workflow whats-next (composite)")
	assert.Contains(t, tableStr, "Priority plan: Core Workflow Test")
	assert.Contains(t, tableStr, "Packaging cue: Package now")
	assert.Contains(t, tableStr, "Materialized view: recovering")
	assert.Contains(t, tableStr, "planned: 2")
	assert.Contains(t, tableStr, "Convergence sessions (active/paused): 1")
	assert.Contains(t, tableStr, "Measure session: CVS-MEASURE-01")
	assert.Contains(t, tableStr, "Observer Recommendations:")
	assert.Contains(t, tableStr, "Kernel steward (ambience):")
	assert.Contains(t, tableStr, "Precondition & Hygiene Auto-Remedy:")
	assert.Contains(t, tableStr, "Kernel fill (no live ATK):")
	assert.Contains(t, tableStr, "Guiding step (hunger):")
	assert.Contains(t, tableStr, "Correspondence (seat):")
	assert.Contains(t, tableStr, "agent_id=agent-worker-1")

	// Test alternate branches: PriorityPlan nil, MeasureSkipReason, MeasureError, Correspondence skip
	buf.Reset()
	outAlt := whatsNextOut{
		Schema:            whatsNextSchema,
		PriorityPlan:      nil,
		MeasureSkipReason: "user skipped",
		Correspondence: &whatsNextCorrespondence{
			SkipReason: "no agent id",
		},
		RemedyRecipes: []string{"recipe 1: run check"},
	}
	err = writeWhatsNextTable(cmd, outAlt)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Priority plan: (none resolved)")
	assert.Contains(t, buf.String(), "Measure: skipped (user skipped)")
	assert.Contains(t, buf.String(), "Deterministic Remedy Recipes:")
	assert.Contains(t, buf.String(), "skip: no agent id")

	// Test MeasureError branch
	buf.Reset()
	outErr := whatsNextOut{
		Schema:       whatsNextSchema,
		MeasureError: "scheduler timeout",
	}
	err = writeWhatsNextTable(cmd, outErr)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Measure error: scheduler timeout")
}

func TestRenderWhatsNextOutput(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	out := whatsNextOut{
		Schema:       whatsNextSchema,
		PriorityPlan: &whatsNextPriorityPlan{ID: "PRI-JSON", Title: "JSON Plan", Status: "active"},
	}

	// 1. JSON
	var bufJSON bytes.Buffer
	cmdJSON := &cobra.Command{}
	cmdJSON.Flags().String("format", "json", "")
	require.NoError(t, cmdJSON.Flags().Set("format", "json"))
	cmdJSON.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufJSON))

	err := renderWhatsNextOutput(cmdJSON, proj.Root, out)
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"schema": "zqk_whats_next_v1"`)
	assert.Contains(t, bufJSON.String(), `"priority_plan"`)

	// 2. YAML
	var bufYAML bytes.Buffer
	cmdYAML := &cobra.Command{}
	cmdYAML.Flags().String("format", "yaml", "")
	require.NoError(t, cmdYAML.Flags().Set("format", "yaml"))
	cmdYAML.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufYAML))

	err = renderWhatsNextOutput(cmdYAML, proj.Root, out)
	require.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "schema: zqk_whats_next_v1")

	// 3. Table default
	var bufTable bytes.Buffer
	cmdTable := &cobra.Command{}
	cmdTable.Flags().String("format", "table", "")
	cmdTable.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufTable))

	err = renderWhatsNextOutput(cmdTable, proj.Root, out)
	require.NoError(t, err)
	assert.Contains(t, bufTable.String(), "Workflow whats-next (composite)")
}

func TestApplyWhatsNextAmbienceAndSeat(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	out := whatsNextOut{
		AgentInstruction: "Original instruction",
		Correspondence: &whatsNextCorrespondence{
			NextActionHint: "test-hint",
		},
	}
	amb := whatsnext.LoadKernelAmbience(proj.Root)
	amb.StewardFocus = "Test Steward Focus"

	applyWhatsNextAmbienceAndSeat(&out, &amb, proj.Root, "test-agent")
	assert.NotNil(t, out.KernelAmbience)
	assert.Contains(t, out.AgentInstruction, "Original instruction")
}

func TestInitWhatsNextContext(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewWhatsNextCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	ctx, sp, out := initWhatsNextContext(proc)
	assert.NotNil(t, ctx)
	assert.NotNil(t, sp)
	assert.Equal(t, whatsNextSchema, out.Schema)
	assert.NotNil(t, out.BacklogCountsByStatus)
}

func TestEnrichWhatsNextRemedies(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-remedy", false, "")

	// 1. Nil out or empty projectRoot
	enrichWhatsNextRemedies(cmd, "", nil)
	enrichWhatsNextRemedies(cmd, proj.Root, nil)

	// 2. KernelAmbience remedies present
	out := &whatsNextOut{
		KernelAmbience: &whatsnext.KernelAmbience{
			RemedyRecipes: []string{"recipe from ambience"},
		},
	}
	enrichWhatsNextRemedies(cmd, proj.Root, out)
	assert.Contains(t, out.RemedyRecipes, "recipe from ambience")
}

func TestRunWhatsNextLite_And_SyncSweep(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-SWEEP-01": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-SWEEP-01",
				objects.FieldKeyTitle:       "Sweep Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
			"BLI-SWEEP-01": {
				objects.FieldKeyKind:            objects.KindBacklogItem,
				objects.FieldKeyID:              "BLI-SWEEP-01",
				objects.FieldKeyTitle:           "Sweep BLI",
				objects.FieldKeyStatus:          objects.ObjectStatusInProgress,
				objects.FieldKeyPriorityPlanRef: "PRI-SWEEP-01",
			},
			"CVS-SWEEP-01": {
				objects.FieldKeyKind:         objects.KindConvergenceSession,
				objects.FieldKeyID:           "CVS-SWEEP-01",
				objects.FieldKeyTitle:        "Convergence Sweep",
				objects.FieldKeyStatus:       "active",
				objects.FieldKeyCurrentPhase: "executing",
			},
		},
	}

	// 1. Test runWhatsNextLite
	cmdLite := NewWhatsNextCmd()
	var bufLite bytes.Buffer
	cmdLite.SetOut(&bufLite)
	cmdLite.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufLite))
	initTestCmdContext(t, cmdLite, proj.Root, store)

	procLite, err := cli.NewProcessor(cmdLite)
	require.NoError(t, err)

	err = runWhatsNextLite(cmdLite, nil, procLite, proj.Root, "PRI-SWEEP-01", "", "", "", true)
	require.NoError(t, err)
	assert.Contains(t, bufLite.String(), "Workflow whats-next")

	// 2. Test runWhatsNextSyncSweep
	cmdSweep := NewWhatsNextCmd()
	var bufSweep bytes.Buffer
	cmdSweep.SetOut(&bufSweep)
	cmdSweep.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufSweep))
	initTestCmdContext(t, cmdSweep, proj.Root, store)

	procSweep, err := cli.NewProcessor(cmdSweep)
	require.NoError(t, err)

	err = runWhatsNextSyncSweep(cmdSweep, nil, procSweep, proj.Root, "PRI-SWEEP-01", "CVS-SWEEP-01", "", "", true)
	require.NoError(t, err)
	assert.Contains(t, bufSweep.String(), "Workflow whats-next")

	// 3. Test runWhatsNext directly via command execution
	cmdRun := NewWhatsNextCmd()
	var bufRun bytes.Buffer
	cmdRun.SetOut(&bufRun)
	cmdRun.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufRun))
	initTestCmdContext(t, cmdRun, proj.Root, store)

	require.NoError(t, cmdRun.Flags().Set("skip-measure", "true"))
	err = runWhatsNext(cmdRun, nil)
	require.NoError(t, err)
	assert.Contains(t, bufRun.String(), "Workflow whats-next")

	// 4. Test runWhatsNext with sync-sweep flag
	cmdRunSync := NewWhatsNextCmd()
	var bufRunSync bytes.Buffer
	cmdRunSync.SetOut(&bufRunSync)
	cmdRunSync.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &bufRunSync))
	initTestCmdContext(t, cmdRunSync, proj.Root, store)

	require.NoError(t, cmdRunSync.Flags().Set("sync-sweep", "true"))
	require.NoError(t, cmdRunSync.Flags().Set("skip-measure", "true"))
	err = runWhatsNext(cmdRunSync, nil)
	require.NoError(t, err)
	assert.Contains(t, bufRunSync.String(), "Workflow whats-next")

	// 5. Test runWhatsNextLite with empty priFlag and cvsFlag
	bufLite.Reset()
	err = runWhatsNextLite(cmdLite, nil, procLite, proj.Root, "", "", "", "", true)
	require.NoError(t, err)

	// 6. Test runWhatsNextLite with unknown priFlag
	bufLite.Reset()
	err = runWhatsNextLite(cmdLite, nil, procLite, proj.Root, "PRI-NON-EXISTENT", "", "", "", true)
	require.NoError(t, err)

	// 7. Test runWhatsNextSyncSweep with empty priFlag
	bufSweep.Reset()
	err = runWhatsNextSyncSweep(cmdSweep, nil, procSweep, proj.Root, "", "", "", "", true)
	require.NoError(t, err)
}

func TestRunWhatsNextSyncSweep_ActiveTask(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-TASK-PLAN": {
				objects.FieldKeyKind:        objects.KindPriorityPlan,
				objects.FieldKeyID:          "PRI-TASK-PLAN",
				objects.FieldKeyTitle:       "Task Priority Plan",
				objects.FieldKeyStatus:      objects.ObjectStatusInProgress,
				objects.FieldKeyActiveOrder: 1,
			},
			"ATK-SYNC-01": {
				objects.FieldKeyKind:               objects.KindAgentTask,
				objects.FieldKeyID:                 "ATK-SYNC-01",
				objects.FieldKeyTitle:              "Active Sync Task",
				objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
				objects.FieldKeyAssigneePersonaRef: "PER-DEV",
				objects.FieldKeyPipelineRef:        "PRI-TASK-PLAN",
				objects.FieldKeyDescription:        "Task Description",
				objects.FieldKeyTaskSteps: []any{
					map[string]any{
						objects.FieldKeyTitle:       "Step 1",
						objects.FieldKeyDescription: "Do step 1",
						objects.FieldKeyStatus:      objects.ObjectStatusPending,
						objects.FieldKeyCommand:     "go test",
					},
				},
			},
		},
	}

	cmd := NewWhatsNextCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	proc, err := cli.NewProcessor(cmd)
	require.NoError(t, err)

	err = runWhatsNextSyncSweep(cmd, nil, proc, proj.Root, "PRI-TASK-PLAN", "", "PER-DEV", "", true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Workflow whats-next")
}
