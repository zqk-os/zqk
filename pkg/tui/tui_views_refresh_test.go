package tui

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/tde"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type mockStorageProvider struct {
	storage.ObjectStorageProvider
	listFn  func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error)
	countFn func(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error)
}

func (m *mockStorageProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, secCtx, storageCtx, filter)
	}
	return &storage.QueryResult{Objects: []map[string]any{}}, nil
}

func (m *mockStorageProvider) Count(ctx context.Context, secCtx *pkgctx.SecurityContext, filter storage.ListFilter) (int, error) {
	if m.countFn != nil {
		return m.countFn(ctx, secCtx, filter)
	}
	return 0, nil
}

func TestCycle133_HelperFunctions(t *testing.T) {
	assert.Equal(t, 0, getIntVal(nil))
	assert.Equal(t, 42, getIntVal(42))
	assert.Equal(t, 100, getIntVal(int64(100)))
	assert.Equal(t, 3, getIntVal(float64(3.14)))
	assert.Equal(t, 0, getIntVal("string"))

	assert.Equal(t, "--", formatTimeVal(nil))
	assert.Equal(t, "--", formatTimeVal(""))
	assert.Equal(t, "--", formatTimeVal("<nil>"))
	assert.Equal(t, "2026-10-23 15:04:05", formatTimeVal("2026-10-23 15:04:05.123456"))
	assert.Equal(t, "short", formatTimeVal("short"))

	assert.Equal(t, "0.50ms", formatDurationMs(0.5))
	assert.Equal(t, "42.5ms", formatDurationMs(42.5))
	assert.Equal(t, "2.50s", formatDurationMs(2500.0))

	assert.Equal(t, "[        ]", generateSparkline(0))
	assert.NotEmpty(t, generateSparkline(5))
	assert.NotEmpty(t, generateSparkline(20))

	assert.Equal(t, "yes", ternary(true, "yes", "no"))
	assert.Equal(t, "no", ternary(false, "yes", "no"))
}

func TestCycle133_TerminalAndRunTUI(t *testing.T) {
	restore, err := SetupRawTerminal(-1)
	assert.Error(t, err)
	assert.Nil(t, restore)

	writeScreen("Header\nLine 1\nLine 2")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := RunTUI(ctx, "", "state", nil, nil)
	require.NoError(t, runErr)
}

func TestCycle133_QueryAndFiltering(t *testing.T) {
	m := NewUIModel("", "state")
	assert.True(t, m.matchesQuery("Alpha", "Beta"))
	m.SearchQuery = "target"
	assert.True(t, m.matchesQuery("some target word", "other"))
	assert.False(t, m.matchesQuery("none", "unmatched"))

	m.Mutations = []state.JournalMutation{
		{ID: "MUT-1", ObjectRef: "target-obj", ChangeType: "update"},
		{ID: "MUT-2", ObjectRef: "other-obj", ChangeType: "create"},
	}
	assert.Len(t, m.GetVisibleMutations(), 1)
	m.SearchQuery = ""
	assert.Len(t, m.GetVisibleMutations(), 2)

	m.SearchQuery = "aud"
	m.AuditEvents = []state.JournalMutation{
		{ID: "AUD-1", ObjectRef: "audit-obj", Actor: "auditor"},
		{ID: "EVT-2", ObjectRef: "other-obj", Actor: "user"},
	}
	assert.Len(t, m.GetVisibleAuditEvents(), 1)
	m.SearchQuery = ""
	assert.Len(t, m.GetVisibleAuditEvents(), 2)
}

func TestCycle133_GetVisibleEntities_Filtered(t *testing.T) {
	m := NewUIModel("", "state")
	m.SearchQuery = "alpha"

	m.RecentBacklog = []PMBacklogRow{
		{ID: "BLI-1", Title: "alpha task"},
		{ID: "BLI-2", Title: "beta task"},
	}
	assert.Len(t, m.GetVisibleBacklog(), 1)

	m.TechnicalDebt = []PMDebtRow{
		{ID: "TD-1", Title: "alpha debt"},
		{ID: "TD-2", Title: "beta debt"},
	}
	assert.Len(t, m.GetVisibleTechnicalDebt(), 1)

	m.CommandMetrics = []CommandMetricRow{
		{ID: "CM-1", CommandName: "alpha-cmd"},
		{ID: "CM-2", CommandName: "beta-cmd"},
	}
	assert.Len(t, m.GetVisibleCommandMetrics(), 1)

	m.SchedulerJobs = []SchedulerJobRow{
		{ID: "JOB-1", Title: "alpha job"},
		{ID: "JOB-2", Title: "beta job"},
	}
	assert.Len(t, m.GetVisibleSchedulerJobs(), 1)

	m.TestCases = []*test.TestCaseModel{
		{ID: "TC-1", Title: "alpha test"},
		{ID: "TC-2", Title: "beta test"},
	}
	assert.Len(t, m.GetVisibleTestCases(), 1)

	m.HealthViolations = []HealthViolationRow{
		{ObjectID: "OBJ-1", Message: "alpha error"},
		{ObjectID: "OBJ-2", Message: "beta error"},
	}
	assert.Len(t, m.GetVisibleHealthViolations(), 1)

	m.PriorityPlans = []PMPlanRow{
		{ID: "PRI-1", Title: "alpha plan", Workstreams: []string{"ws1"}},
		{ID: "PRI-2", Title: "beta plan"},
	}
	assert.Len(t, m.GetVisiblePriorityPlans(), 1)

	m.Blockers = []PMBlockerRow{
		{ID: "BLK-1", Title: "alpha risk"},
		{ID: "BLK-2", Title: "beta risk"},
	}
	assert.Len(t, m.GetVisibleBlockers(), 1)
}

func TestCycle133_RowCountAllTabs(t *testing.T) {
	m := NewUIModel("", "state")
	m.Mutations = []state.JournalMutation{{ID: "1"}}
	m.AuditEvents = []state.JournalMutation{{ID: "2"}}
	m.InboxItems = []InboxItemRow{{ID: "3"}}
	m.RecentBacklog = []PMBacklogRow{{ID: "4"}}
	m.CommandMetrics = []CommandMetricRow{{ID: "5"}}
	m.SchedulerJobs = []SchedulerJobRow{{ID: "6"}}
	m.TestCases = []*test.TestCaseModel{{ID: "7"}}
	m.HealthViolations = []HealthViolationRow{{ObjectID: "8"}}

	m.ActiveTab = TabState
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabAudit
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabSwarm
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabPM
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabMetrics
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabScheduler
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabQA
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = TabHealth
	assert.Equal(t, 1, m.GetCurrentRowCount())
	m.ActiveTab = 99
	assert.Equal(t, 0, m.GetCurrentRowCount())
}

func TestCycle133_DetailModal_StateAuditSwarm(t *testing.T) {
	m := NewUIModel("", "state")
	m.SelectedIndex = -1
	m.OpenSelectedItemDetail()
	assert.Nil(t, m.DetailModal)

	m.SelectedIndex = 0
	m.ActiveTab = TabState
	m.Mutations = []state.JournalMutation{
		{ID: "M-1", ChangeType: "create", ObjectRef: "obj:1", DiffSummary: "summary", CreatedAt: time.Now().Unix()},
	}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "state_mutation", m.DetailModal.Kind)

	m.ActiveTab = TabAudit
	m.AuditEvents = []state.JournalMutation{
		{ID: "A-1", Operation: "read", ObjectRef: "obj:2", CreatedBy: "admin", DiffSummary: "diff", CreatedAt: 0},
	}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "audit_event", m.DetailModal.Kind)

	m.ActiveTab = TabSwarm
	m.InboxItems = []InboxItemRow{
		{ID: "I-1", Type: "tde_envelope", Sender: "agent-1", Target: "operator", Status: "pending", Summary: "staged"},
	}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "tde_envelope", m.DetailModal.Kind)
}

func TestCycle133_DetailModal_PMSections(t *testing.T) {
	m := NewUIModel("", "pm")
	m.ActiveTab = TabPM
	m.PriorityPlans = []PMPlanRow{{ID: "P-1", Title: "Plan", Status: "active", Workstreams: []string{"w1"}, BLICount: 2}}
	m.Blockers = []PMBlockerRow{{ID: "B-1", Title: "Blocker", Severity: "P0", Status: "open", Impact: "blocks all"}}
	m.RecentBacklog = []PMBacklogRow{{ID: "BLI-1", Title: "Backlog", Status: "in_progress", Priority: "P1", ClaimedBy: "dev", PlanRef: "P-1"}}
	m.TechnicalDebt = []PMDebtRow{{ID: "TD-1", Title: "Debt", Category: "hygiene", Priority: "P2", Status: "identified"}}

	m.SelectedIndex = 0
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindPriorityPlan, m.DetailModal.Kind)

	m.SelectedIndex = 1
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindRiskBlocker, m.DetailModal.Kind)

	m.SelectedIndex = 2
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindBacklogItem, m.DetailModal.Kind)

	m.SelectedIndex = 3
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindTechnicalDebt, m.DetailModal.Kind)
}

func TestCycle133_DetailModal_MetricsSchedQAHealth(t *testing.T) {
	m := NewUIModel("", "metrics")
	m.SelectedIndex = 0

	m.ActiveTab = TabMetrics
	m.CommandMetrics = []CommandMetricRow{{ID: "CM-1", CommandName: "test-cmd", ExecCount: 5, AvgDuration: "12ms", LastRunAt: "2026-10-23", Status: "ok"}}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindCommandMetric, m.DetailModal.Kind)

	m.ActiveTab = TabScheduler
	m.SchedulerJobs = []SchedulerJobRow{{ID: "SCH-1", Title: "Job", Status: "idle", Command: "run", CommandArgs: []string{"--flag"}, LastError: "err"}}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindSchedulerJob, m.DetailModal.Kind)

	m.ActiveTab = TabQA
	m.TestCases = []*test.TestCaseModel{
		{
			ID: "TC-1", Title: "Test Case", Status: "passed", Scope: "unit", Category: "pkg", PathOrID: "pkg/test.go",
			CompletedCriteria: 1, TotalCriteria: 1,
			Lineage: &test.LineageChain{
				IsIntact:     true,
				RootObject:   &test.LineageNode{Kind: "goal", ID: "G-1", Title: "Goal"},
				Requirements: []*test.LineageNode{{Kind: "req", ID: "R-1", Title: "Req"}},
				BacklogItems: []*test.LineageNode{{Kind: "bli", ID: "B-1", Title: "BLI"}},
			},
			Criteria: []*test.CriterionState{{ID: "CR-1", Description: "Crit", Status: "passed", LastSatisfied: time.Now()}},
		},
	}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindTestCase, m.DetailModal.Kind)

	m.ActiveTab = TabHealth
	m.HealthViolations = []HealthViolationRow{
		{Tier: 1, Severity: "BLOCKER", Kind: "policy", ObjectID: "POL-1", Category: "security", Message: "fail", AutoFixable: true, Path: "cas/path"},
	}
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "policy", m.DetailModal.Kind)
}

func TestCycle133_RefreshActiveTab_AllTabs(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "state")
	ctx := context.Background()
	sp := &mockStorageProvider{ObjectStorageProvider: storage.NewNoopObjectStorage()}
	sec := pkgctx.NewSystemSecurityContext()

	tabs := []int{TabState, TabAudit, TabSwarm, TabPM, TabMetrics, TabScheduler, TabQA, TabHealth}
	for _, tab := range tabs {
		m.ActiveTab = tab
		m.RefreshActiveTab(ctx, sp, sec)
		m.RefreshActiveTab(ctx, nil, nil)
	}
}

func TestCycle133_RefreshObjects_And_Streams(t *testing.T) {
	tmpDir := t.TempDir()
	procGoalDir := filepath.Join(tmpDir, paths.ProcessDir, "goal")
	require.NoError(t, fileutil.MkdirAll(procGoalDir, 0755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(procGoalDir, "GOAL-1.yaml"), []byte("id: GOAL-1\n"), 0644))

	streamDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	require.NoError(t, fileutil.MkdirAll(streamDir, 0755))
	require.NoError(t, fileutil.WriteFile(filepath.Join(streamDir, "001.jsonl"), []byte("{}\n"), 0644))

	m := NewUIModel(tmpDir, "state")
	m.RefreshObjects()
	assert.Equal(t, 1, m.Hygiene.ProcessObjectCount)
	assert.Equal(t, 1, m.Hygiene.StreamFileCount)
	assert.Equal(t, 1, m.Hygiene.ActiveStreams)

	m.ProjectRoot = ""
	m.RefreshObjects()
	m.RefreshMutations()
	m.RefreshAuditEvents()
}

func TestCycle133_RefreshPM_Populated(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	sp := &mockStorageProvider{
		ObjectStorageProvider: storage.NewNoopObjectStorage(),
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			switch filter.Kind {
			case objects.KindMission:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyTitle: "Mission Alpha"}}}, nil
			case objects.KindVision:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyTitle: "Vision Alpha"}}}, nil
			case objects.KindGoal:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "G-1", objects.FieldKeyTitle: "Goal 1", objects.FieldKeyStatus: "active"}}}, nil
			case objects.KindWorkstream:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "WS-1", objects.FieldKeyTitle: "Workstream 1", objects.FieldKeyStatus: "active"}}}, nil
			case objects.KindBacklogItem:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "BLI-1", objects.FieldKeyTitle: "BLI 1", objects.FieldKeyStatus: "in_progress", "claimed_by": "dev-1", "priority_tier": "P0", objects.FieldKeyPriorityPlanRef: "PRI-1"},
					{objects.FieldKeyID: "BLI-2", objects.FieldKeyTitle: "BLI 2", objects.FieldKeyStatus: "complete", "priority": "P2"},
					{objects.FieldKeyID: "BLI-3", objects.FieldKeyTitle: "BLI 3", objects.FieldKeyStatus: "draft"},
					{objects.FieldKeyID: "BLI-4", objects.FieldKeyTitle: "BLI 4", objects.FieldKeyStatus: "blocked"},
					{objects.FieldKeyID: "BLI-5", objects.FieldKeyTitle: "BLI 5", objects.FieldKeyStatus: "planned"},
				}}, nil
			case objects.KindPriorityPlan:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "PRI-1", objects.FieldKeyTitle: "Plan 1", objects.FieldKeyStatus: "in_progress", objects.FieldKeyWorkstreamRefs: []any{"WS-1"}}}}, nil
			case objects.KindRequirement:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "REQ-1", objects.FieldKeyTitle: "Req 1", objects.FieldKeyStatus: "active"}}}, nil
			case objects.KindRiskBlocker:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "BLK-1", objects.FieldKeyTitle: "Risk 1", "severity": "P0", objects.FieldKeyStatus: "open", "impact": "critical"}}}, nil
			case objects.KindTechnicalDebt:
				return &storage.QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "TD-1", objects.FieldKeyTitle: "Debt 1", "debt_category": "test", objects.FieldKeyStatus: "open", "priority": "P1"}}}, nil
			}
			return &storage.QueryResult{}, nil
		},
	}

	m := NewUIModel("", "pm")
	m.RefreshPM(ctx, nil, nil)
	assert.Empty(t, m.MissionTitle)

	m.RefreshPM(ctx, sp, sec)
	assert.Equal(t, "Mission Alpha", m.MissionTitle)
	assert.Equal(t, "Vision Alpha", m.VisionTitle)
	assert.Len(t, m.Goals, 1)
	assert.Len(t, m.Workstreams, 1)
	assert.Len(t, m.PriorityPlans, 1)
	assert.Equal(t, 1, m.PriorityPlans[0].BLICount)
	assert.Len(t, m.Requirements, 1)
	assert.Len(t, m.Blockers, 1)
	assert.Len(t, m.TechnicalDebt, 1)
	assert.Equal(t, 5, m.BacklogSummary.Total)
}

func TestCycle133_RefreshMetrics_And_Scheduler(t *testing.T) {
	ctx := context.Background()
	sec := pkgctx.NewSystemSecurityContext()

	sp := &mockStorageProvider{
		ObjectStorageProvider: storage.NewNoopObjectStorage(),
		listFn: func(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
			switch filter.Kind {
			case objects.KindCommandMetric:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "CM-1", "command_name": "cmd1", "execution_count": 10, "duration": "5ms", "last_executed_at": "2026-10-23 10:00:00", objects.FieldKeyStatus: "ok"},
				}}, nil
			case objects.KindSchedulerHealthMetric:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "SH-1", "heartbeat_at": "2026-10-23 10:00:00", "total_executions": 50, "failure_count": 0, objects.FieldKeyStatus: "healthy"},
					{objects.FieldKeyID: "SH-2", "last_seen": "2026-10-23 09:59:00", "collection_count": 30, "failures": 2, "status": "degraded"},
				}}, nil
			case objects.KindFileLockMetric:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "FLM-1", "target_kind": "bli", "contention_count": 0, "lock_duration": "1ms", objects.FieldKeyStatus: "healthy"},
				}}, nil
			case objects.KindAuditAggregationMetric:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "AAM-1", "events_processed": 100, "aggregation_window": "5m", objects.FieldKeyStatus: "ok"},
				}}, nil
			case objects.KindSchedulerJob:
				return &storage.QueryResult{Objects: []map[string]any{
					{objects.FieldKeyID: "SCH-1", objects.FieldKeyTitle: "Job 1", "category": "maintenance", "schedule": "@every 5m", objects.FieldKeyStatus: "active"},
				}}, nil
			}
			return &storage.QueryResult{}, nil
		},
	}

	m := NewUIModel("", "metrics")
	m.RefreshMetrics(ctx, nil, nil)
	m.RefreshScheduler(ctx, nil, nil)
	assert.Empty(t, m.CommandMetrics)

	m.RefreshMetrics(ctx, sp, sec)
	m.RefreshScheduler(ctx, sp, sec)
	assert.Len(t, m.CommandMetrics, 1)
	assert.Len(t, m.SchedulerHealth, 2)
	assert.Len(t, m.LockMetrics, 1)
	assert.Len(t, m.QualityMetrics, 1)
	assert.Len(t, m.SchedulerJobs, 1)
}

func TestCycle133_RefreshHealth_FullDiagnostics(t *testing.T) {
	tmpDir := t.TempDir()

	cacheDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.CacheDir)
	require.NoError(t, fileutil.MkdirAll(cacheDir, 0755))
	cacheData := map[string]any{
		"version": "1.0",
		"updated": time.Now().Add(-5 * time.Minute),
		"by_kind": map[string]any{
			"policy": map[string]any{
				"path_prefix": "cas/store",
				"entries": []any{
					map[string]any{
						"id":   "POL-1",
						"path": "policies/pol1.yaml",
						"issues": []any{
							map[string]any{"tier": 1, "category": "sec", "message": "tier1 issue", "auto_fixable": false},
							map[string]any{"tier": 2, "category": "perf", "message": "tier2 issue", "auto_fixable": true},
							map[string]any{"tier": 3, "category": "hygiene", "message": "tier3 issue", "auto_fixable": false},
							map[string]any{"tier": 4, "category": "info", "message": "tier4 issue", "auto_fixable": false},
						},
					},
				},
			},
		},
	}
	rawCache, err := json.Marshal(cacheData)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(filepath.Join(cacheDir, paths.ValidationCacheFile), rawCache, 0644))

	m := NewUIModel(tmpDir, "health")
	m.RefreshHealth()
	assert.Len(t, m.HealthViolations, 4)
	assert.Equal(t, "BLOCKED", m.HealthSummary.OverallStatus)
	assert.Equal(t, 1, m.HealthSummary.AutoFixableCount)

	assert.False(t, m.TriggerActionCenter("unknown_key"))
	assert.True(t, m.TriggerActionCenter("c"))

	assert.False(t, m.TriggerScheduledJob("", 0))
	assert.True(t, m.TriggerScheduledJob("SCH-test", 0))
	assert.True(t, m.TriggerScheduledJob("SCH-delayed", 1))

	m.ProjectRoot = ""
	m.RefreshHealth()
	assert.False(t, m.TriggerActionCenter("c"))
	assert.False(t, m.TriggerScheduledJob("SCH-test", 0))
}

func TestCycle133_RefreshHealth_DiagnosticsLogProcessing(t *testing.T) {
	tmpDir := t.TempDir()
	schedDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))

	diagPath := filepath.Join(schedDir, "diagnostics.jsonl")
	evCompleted := `{"job_id":"SCH-test-comp","status":"completed"}` + "\n"
	evFailed := `{"job_id":"SCH-test-fail","status":"failed"}` + "\n"
	evStarted := `{"job_id":"SCH-test-proc","status":"started"}` + "\n"
	require.NoError(t, fileutil.WriteFile(diagPath, []byte(evCompleted+evFailed+evStarted), 0644))

	m := NewUIModel(tmpDir, "health")
	m.ActionItems = []ActionCenterItem{
		{Key: "1", JobID: "SCH-test-comp", IsTriggered: true},
		{Key: "2", JobID: "SCH-test-fail", IsTriggered: true},
		{Key: "3", JobID: "SCH-test-proc", IsTriggered: true},
		{Key: "4", JobID: "SCH-test-unknown", IsTriggered: true, TriggeredAt: time.Now()},
	}
	m.RefreshHealth()
	assert.Equal(t, "completed", m.ActionItems[0].Status)
	assert.Equal(t, "failed", m.ActionItems[1].Status)
	assert.Equal(t, "processing", m.ActionItems[2].Status)
	assert.Equal(t, "processing", m.ActionItems[3].Status)
}

func TestCycle133_InboxLifecycle_StagedAndPeer(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "swarm")

	assert.False(t, m.AcknowledgeInboxItem(""))
	assert.False(t, m.RespondInboxItem("", "msg"))

	wal, err := tde.NewStagingWAL(tmpDir)
	require.NoError(t, err)
	defer wal.Close()

	env := tde.Envelope{
		ID:         "ENV-CYC133-01",
		Kind:       "policy",
		TargetID:   "pol-1",
		Operation:  "update",
		PayloadB64: "dGVzdA==",
		ExecuteAt:  time.Now().Add(1 * time.Hour),
	}
	require.NoError(t, wal.Stage(env))
	require.NoError(t, wal.Sync())

	m.RefreshInbox()
	visible := m.GetVisibleInboxItems()
	found := false
	for _, it := range visible {
		if it.ID == env.ID {
			found = true
			break
		}
	}
	assert.True(t, found)

	assert.True(t, m.AcknowledgeInboxItem(env.ID))

	require.NoError(t, datacell.WriteAgentChatChannelConfig(tmpDir, datacell.AgentChatChannelConfig{
		SchemaVersion: datacell.AgentChatChannelSchemaVersion,
		Enabled:       true,
		DeliveryMode:  datacell.DeliveryModeLog,
		FeedID:        "AGF-test",
	}))

	peerEvent, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
		ProjectRoot: tmpDir,
		Message:     "Worker asking guidance",
		AgentID:     "worker-1",
		ToAgentID:   "operator",
		Sender:      agentfeed.FeedSenderHumanSteer,
		EventType:   agentfeed.FeedEventTypeSteering,
		SelfACK:     true,
	})
	require.NoError(t, err)

	m.InboxItems = []InboxItemRow{{ID: peerEvent.EventID, Sender: "worker-1"}}
	assert.True(t, m.RespondInboxItem(peerEvent.EventID, "Proceed with cycle 133"))
}

func TestCycle133_Views_MetricsCard_WithTSDB(t *testing.T) {
	m := NewUIModel("", "metrics")
	m.Width = 120
	m.Height = 40
	m.TSDB = &state.TSDBTelemetry{
		HygieneStats: state.TSDBHygieneStats{
			OpenFileDescriptors: 25,
			MaxFileDescriptors:  1024,
			FDUsagePercent:      2.44,
			Status:              "HEALTHY",
			StorageSizeStr:      "50MB",
			TotalStorageFiles:   120,
			StaleLocksCount:     0,
			DraftObjectsCount:   0,
		},
		KindVolumes: []state.TSDBKindVolume{
			{Kind: "goal", CurrentCount: 5},
			{Kind: "req", CurrentCount: 15},
			{Kind: "bli", CurrentCount: 30},
			{Kind: "plan", CurrentCount: 3},
			{Kind: "crit", CurrentCount: 45},
		},
		StreamRates: []state.TSDBStreamRate{
			{StreamName: "change_journal_entry", TotalFiles: 200},
			{StreamName: "audit_event", TotalFiles: 500},
			{StreamName: "agent_instruction", TotalFiles: 40},
			{StreamName: "process_lifecycle", TotalFiles: 10},
		},
		TopCommands: []state.TSDBCommandMetric{
			{Command: "zqk ui", Invocations: 10, AvgLatencyMs: 12.5, Failures: 0},
			{Command: "zqk system check", Invocations: 5, AvgLatencyMs: 150.0, Failures: 1},
		},
		JobSummaries: []state.TSDBJobSummary{
			{JobID: "SCH-1", Executions: 20, Successes: 19, Failures: 1, AvgDurationMs: 45.0, MinDurationMs: 10.0, MaxDurationMs: 100.0, Sparkline: "▲ SPIKE", LastRunAt: time.Now()},
			{JobID: "SCH-2", Executions: 50, Successes: 50, Failures: 0, AvgDurationMs: 5.0, MinDurationMs: 1.0, MaxDurationMs: 12.0, Sparkline: "▼ DROP", LastRunAt: time.Now()},
			{JobID: "SCH-3", Executions: 10, Successes: 10, Failures: 0, AvgDurationMs: 2.0, MinDurationMs: 1.0, MaxDurationMs: 3.0, Sparkline: "STABLE", LastRunAt: time.Time{}},
		},
	}
	m.SchedulerHealth = []SchedulerHealthRow{
		{ID: "SCH-H1", HeartbeatAt: "10:00:00", Executions: 10, Failures: 0},
		{ID: "SCH-H2", HeartbeatAt: "--", Executions: 0, Failures: 1},
	}

	out := Render(m)
	assert.Contains(t, out, "File Descriptors")
	assert.Contains(t, out, "KERNEL OBJECT INVENTORY")
	assert.Contains(t, out, "STREAM INGESTION VOLUME")
	assert.Contains(t, out, "TOP CLI COMMAND VELOCITY")
	assert.Contains(t, out, "SCHEDULER JOB LATENCY & RELIABILITY")
	assert.Contains(t, out, "SCHEDULER HEALTH TELEMETRY")
}

func TestCycle133_Views_QATab_AllStates(t *testing.T) {
	m := NewUIModel("", "qa")
	m.Width = 110
	m.Height = 35

	m.QASummary = QASummaryRow{
		TotalTestCases:    0,
		DoDCompliant:      false,
		IntactChains:      0,
		UnboundCriteria:   2,
		SatisfiedCriteria: 0,
		TotalCriteria:     0,
		TotalBLICount:     0,
		VerifiedBLICount:  0,
	}
	m.UnboundCriteria = []*test.UnboundCriterionModel{
		{ID: "CR-UNBOUND-1", Title: "Unbound criteria item", Status: "open"},
	}
	m.RecentQAEvents = []test.LifecycleEventSummary{
		{Timestamp: time.Now(), Message: "Criteria CR-1 satisfied"},
	}

	outEmpty := Render(m)
	assert.Contains(t, outEmpty, "0% (No Test Suites)")

	m.TestCases = []*test.TestCaseModel{
		{ID: "TC-0", Title: "Placeholder", Status: "passed"},
	}
	outWithUnbound := Render(m)
	assert.Contains(t, outWithUnbound, "UNBOUND CRITERIA")

	m.QASummary = QASummaryRow{
		TotalTestCases:    2,
		DoDCompliant:      true,
		IntactChains:      2,
		UnboundCriteria:   0,
		SatisfiedCriteria: 5,
		TotalCriteria:     5,
		TotalBLICount:     3,
		VerifiedBLICount:  3,
	}
	m.TestCases = []*test.TestCaseModel{
		{ID: "TC-1", Title: "Passed suite", Status: "passed", CompletedCriteria: 3, TotalCriteria: 3, Lineage: &test.LineageChain{IsIntact: true}},
		{ID: "TC-2", Title: "Failed suite", Status: "failed", CompletedCriteria: 0, TotalCriteria: 2, Lineage: nil},
	}
	outPopulated := Render(m)
	assert.Contains(t, outPopulated, "100% Intact")
	assert.Contains(t, outPopulated, "TC-1")
	assert.Contains(t, outPopulated, "TC-2")
}

func TestCycle133_Views_HeaderFooterProfiles(t *testing.T) {
	m := NewUIModel("", "state")
	widths := []int{60, 80, 100, 120, 140}
	profiles := []string{ProfileNewb, ProfilePro, ProfileJedi}
	tabs := []int{TabState, TabScheduler, TabQA, TabSwarm}

	for _, p := range profiles {
		m.SetEditorProfile(p)
		for _, w := range widths {
			m.Width = w
			for _, tab := range tabs {
				m.ActiveTab = tab
				out := Render(m)
				assert.NotEmpty(t, out)
			}
		}
	}
}

func TestCycle133_Views_DynamicMessageAndSearchBanners(t *testing.T) {
	m := NewUIModel("", "state")
	m.Width = 100

	msgs := []string{
		"✓ Operation succeeded cleanly",
		"⚡ Background mutation queued",
		"⏳ Waiting for lock acquisition",
		"✗ Failed to compile test target",
		"Regular notification info",
	}
	for _, msg := range msgs {
		m.DynamicMessage = msg
		out := Render(m)
		assert.Contains(t, out, "MESSAGE:")
	}

	m.IsSearching = true
	m.SearchBuffer = "searchterm"
	outSearching := Render(m)
	assert.Contains(t, outSearching, "SEARCH:")

	m.IsSearching = false
	m.SearchQuery = "activefilter"
	m.Mutations = []state.JournalMutation{{ID: "1", ObjectRef: "activefilter"}}
	outFilter := Render(m)
	assert.Contains(t, outFilter, "activefilter")
}

func TestCycle133_Views_SwarmInboxTable(t *testing.T) {
	m := NewUIModel("", "swarm")
	m.Width = 100

	m.InboxItems = nil
	outEmpty := Render(m)
	assert.Contains(t, outEmpty, "No unacknowledged correspondence")

	m.InboxItems = []InboxItemRow{
		{ID: "INB-1", Type: "correspondence", Sender: "agent-a", Target: "operator", Status: "acked", Summary: "Short summary"},
		{ID: "INB-2", Type: "tde_envelope", Sender: "agent-b", Target: "operator", Status: "committed", Summary: strings.Repeat("Long summary text exceeding forty chars ", 2)},
	}
	outPopulated := Render(m)
	assert.Contains(t, outPopulated, "INB-1")
	assert.Contains(t, outPopulated, "INB-2")
}

func TestCycle133_Views_SchedulerTabEmptyAndPopulated(t *testing.T) {
	m := NewUIModel("", "sched")
	m.ActiveTab = TabScheduler
	m.Width = 90

	outEmpty := Render(m)
	assert.Contains(t, outEmpty, "No scheduler jobs registered")

	m.SearchQuery = "nonexistent"
	outEmptySearch := Render(m)
	assert.Contains(t, outEmptySearch, "No scheduler jobs matching search query")

	m.SearchQuery = ""
	m.SchedulerJobs = []SchedulerJobRow{
		{ID: "J-1", Schedule: "@every 1m", Status: "active"},
		{ID: "J-2", Schedule: "@every 5m", Status: "failed"},
		{ID: "J-3", Schedule: "@hourly", Status: "paused"},
	}
	outJobs := Render(m)
	assert.Contains(t, outJobs, "J-1")
	assert.Contains(t, outJobs, "J-2")
	assert.Contains(t, outJobs, "J-3")
}

func TestCycle133_Views_StateAndAuditTabs_EmptyAndPaused(t *testing.T) {
	m := NewUIModel("", "state")
	m.Width = 90

	m.AutoScroll = false
	m.ScrollOffset = 5
	outStateEmpty := Render(m)
	assert.Contains(t, outStateEmpty, "PAUSED: +5")
	assert.Contains(t, outStateEmpty, "No state mutations recorded")

	m.ActiveTab = TabAudit
	outAuditEmpty := Render(m)
	assert.Contains(t, outAuditEmpty, "PAUSED: +5")
	assert.Contains(t, outAuditEmpty, "audit_event")

	m.AuditEvents = []state.JournalMutation{
		{ID: "A-1", ObjectRef: "obj-ref-1", CreatedBy: "long_actor_name_for_truncation_testing", Operation: "WRITE", DiffSummary: "payload"},
	}
	m.AutoScroll = true
	m.ScrollOffset = 0
	outAuditPop := Render(m)
	assert.Contains(t, outAuditPop, "payload")
}

func TestCycle133_HandleInput_ModalOverlayActions(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "sched")

	m.DetailModal = &ItemDetailModel{Kind: objects.KindSchedulerJob, ID: "SCH-job-1"}
	assert.False(t, handleInput(m, []byte{'t'}))
	assert.False(t, handleInput(m, []byte{'d'}))

	m.DetailModal = &ItemDetailModel{Kind: objects.KindTestCase, ID: "TC-1"}
	assert.False(t, handleInput(m, []byte{'t'}))

	m.DetailModal = &ItemDetailModel{Kind: "inbox_item", ID: "INB-1"}
	assert.False(t, handleInput(m, []byte{'a'}))
	assert.Nil(t, m.DetailModal)

	m.DetailModal = &ItemDetailModel{Kind: "correspondence", ID: "INB-2"}
	assert.False(t, handleInput(m, []byte{'r'}))
	assert.Nil(t, m.DetailModal)

	m.DetailModal = &ItemDetailModel{Kind: "other", ID: "O-1"}
	assert.False(t, handleInput(m, []byte{KeyEsc}))
	assert.Nil(t, m.DetailModal)

	m.DetailModal = &ItemDetailModel{Kind: "other", ID: "O-2"}
	assert.True(t, handleInput(m, []byte{KeyCtrlC}))
}

func TestCycle133_HandleInput_SearchMode(t *testing.T) {
	m := NewUIModel("", "state")
	m.IsSearching = true

	assert.False(t, handleInput(m, []byte{'a'}))
	assert.False(t, handleInput(m, []byte{'b'}))
	assert.Equal(t, "ab", m.SearchBuffer)

	assert.False(t, handleInput(m, []byte{KeyBackspace}))
	assert.Equal(t, "a", m.SearchBuffer)

	assert.False(t, handleInput(m, []byte{KeyEnter}))
	assert.False(t, m.IsSearching)
	assert.Equal(t, "a", m.SearchQuery)

	assert.False(t, handleInput(m, []byte{KeyEsc}))
	assert.Equal(t, "", m.SearchQuery)

	m.IsSearching = true
	m.SearchBuffer = "hello"
	assert.False(t, handleInput(m, []byte{KeyCtrlC}))
	assert.Equal(t, "", m.SearchBuffer)

	m.IsSearching = true
	assert.False(t, handleInput(m, []byte{KeyEsc}))
	assert.False(t, m.IsSearching)
}

func TestCycle133_HandleInput_NavigationAndTriggers(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "state")
	m.Mutations = []state.JournalMutation{{ID: "1"}, {ID: "2"}, {ID: "3"}}

	assert.False(t, handleInput(m, []byte{'/'}))
	assert.True(t, m.IsSearching)
	m.IsSearching = false

	assert.False(t, handleInput(m, []byte{'n'}))
	assert.False(t, handleInput(m, []byte{'N'}))
	assert.False(t, handleInput(m, []byte{'g'}))
	assert.False(t, handleInput(m, []byte{'G'}))
	assert.False(t, handleInput(m, []byte{KeySpace}))
	assert.False(t, handleInput(m, []byte{'z'}))
	assert.False(t, handleInput(m, []byte{'r'}))
	assert.False(t, handleInput(m, []byte{'j'}))
	assert.False(t, handleInput(m, []byte{'k'}))

	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, 'H'}))
	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, 'F'}))
	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodeArrowUp}))
	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodeArrowDown}))
	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodePageUp}))
	assert.False(t, handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodePageDown}))

	assert.True(t, handleInput(m, []byte{'q'}))
	assert.True(t, handleInput(m, []byte{'Q'}))
}

func TestCycle133_HandleInput_TabSpecificActions(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "health")
	m.ActiveTab = TabHealth
	m.RefreshHealth()
	assert.False(t, handleInput(m, []byte{'c'}))

	m.ActiveTab = TabScheduler
	m.SchedulerJobs = []SchedulerJobRow{{ID: "SCH-1"}}
	assert.False(t, handleInput(m, []byte{'t'}))
	assert.False(t, handleInput(m, []byte{'d'}))

	m.ActiveTab = TabQA
	assert.False(t, handleInput(m, []byte{'t'}))

	m.ActiveTab = TabSwarm
	m.InboxItems = []InboxItemRow{{ID: "INB-1"}}
	assert.False(t, handleInput(m, []byte{'a'}))
	assert.False(t, handleInput(m, []byte{'r'}))
}
