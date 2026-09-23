package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/test"
	"github.com/zqk-os/zqk/cmd/zqk/ui/tds"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestNewUICmd(t *testing.T) {
	cmd := NewUICmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "ui", cmd.Name())
	assert.Contains(t, cmd.Aliases, "dashboard")
	assert.Contains(t, cmd.Aliases, "console")

	tabFlag := cmd.Flag("tab")
	require.NotNil(t, tabFlag)
	assert.Equal(t, "seismograph", tabFlag.DefValue)
}

func TestAnsiConstants(t *testing.T) {
	assert.Equal(t, "\033[?1049h", AnsiAltBufferEnter)
	assert.Equal(t, "\033[?1049l", AnsiAltBufferExit)
	assert.Equal(t, "\033[2J", AnsiClearScreen)
	assert.Equal(t, "\033[H", AnsiHomeCursor)
	assert.Equal(t, "\033[?25l", AnsiHideCursor)
	assert.Equal(t, "\033[?25h", AnsiShowCursor)
	assert.Equal(t, "\033[K", AnsiClearToEOL)
	assert.Equal(t, "\r\n", CRLF)

	assert.Equal(t, byte(3), KeyCtrlC)
	assert.Equal(t, byte('\t'), KeyTab)
	assert.Equal(t, byte(27), KeyEsc)
	assert.Equal(t, byte(' '), KeySpace)
}

func TestUIModel_NavigationAndScrolling(t *testing.T) {
	m := NewUIModel("", "state")
	assert.Equal(t, TabState, m.ActiveTab)
	assert.True(t, m.AutoScroll)
	assert.Equal(t, 0, m.ScrollOffset)

	// Tab cycle forward through all 8 tabs
	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabAudit, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabSwarm, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabPM, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabMetrics, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabScheduler, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabQA, m.ActiveTab)

	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabHealth, m.ActiveTab)

	// Cycle back to 0
	assert.False(t, handleInput(m, []byte{KeyTab}))
	assert.Equal(t, TabState, m.ActiveTab)

	// Direct numeric key selection (1-8)
	handleInput(m, []byte{'2'})
	assert.Equal(t, TabAudit, m.ActiveTab)

	handleInput(m, []byte{'3'})
	assert.Equal(t, TabSwarm, m.ActiveTab)

	handleInput(m, []byte{'4'})
	assert.Equal(t, TabPM, m.ActiveTab)

	handleInput(m, []byte{'5'})
	assert.Equal(t, TabMetrics, m.ActiveTab)

	handleInput(m, []byte{'6'})
	assert.Equal(t, TabScheduler, m.ActiveTab)

	handleInput(m, []byte{'7'})
	assert.Equal(t, TabQA, m.ActiveTab)

	handleInput(m, []byte{'8'})
	assert.Equal(t, TabHealth, m.ActiveTab)

	handleInput(m, []byte{'1'})
	assert.Equal(t, TabState, m.ActiveTab)

	// Shift+Tab backward cycle
	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodeShiftTab})
	assert.Equal(t, TabHealth, m.ActiveTab)

	// Scroll controls
	handleInput(m, []byte{'k'}) // Scroll up
	assert.False(t, m.AutoScroll)
	assert.Equal(t, 1, m.ScrollOffset)

	handleInput(m, []byte{'k'})
	assert.Equal(t, 2, m.ScrollOffset)

	handleInput(m, []byte{'j'}) // Scroll down
	assert.Equal(t, 1, m.ScrollOffset)

	handleInput(m, []byte{'j'})
	assert.Equal(t, 0, m.ScrollOffset)
	assert.True(t, m.AutoScroll) // Returning to 0 re-enables auto-scroll

	// Toggle pause with Space
	handleInput(m, []byte{KeySpace})
	assert.False(t, m.AutoScroll)

	handleInput(m, []byte{KeySpace})
	assert.True(t, m.AutoScroll)

	// Arrow keys
	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodeArrowUp})
	assert.Equal(t, 1, m.ScrollOffset)

	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodeArrowDown})
	assert.Equal(t, 0, m.ScrollOffset)

	// Page Up / Down
	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodePageUp})
	assert.Equal(t, 10, m.ScrollOffset)

	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, SeqCodePageDown})
	assert.Equal(t, 0, m.ScrollOffset)

	// Exit keys
	assert.True(t, handleInput(m, []byte{'q'}))
	assert.True(t, handleInput(m, []byte{'Q'}))
	assert.True(t, handleInput(m, []byte{KeyEsc}))
	assert.True(t, handleInput(m, []byte{KeyCtrlC}))
}

func TestRender_AllSixTabs(t *testing.T) {
	m := NewUIModel("", "state")
	m.Width = 110
	m.Height = 35

	// Tab 1 Data: State stream
	m.Mutations = []state.JournalMutation{
		{
			ID:          "CJE-101",
			ChangeType:  "object_update",
			ObjectRef:   "backlog_item:BLI-001",
			DiffSummary: "status changed to in_progress",
			CreatedAt:   time.Now().Unix(),
		},
	}

	// Tab 2 Data: Audit events stream
	m.AuditEvents = []state.JournalMutation{
		{
			ID:          "AUD-201",
			ChangeType:  "COMMAND_EXECUTION",
			ObjectRef:   "command_spec:zqk-state-stream",
			DiffSummary: "zqk state stream --dashboard executed",
			CreatedBy:   "ACC-SYSTEM",
			Actor:       "ACC-SYSTEM",
			Operation:   "EXECUTE",
			CreatedAt:   time.Now().Unix(),
		},
	}

	// Tab 3 Data: Swarm
	m.SwarmData = map[string]any{
		"throughput_hint":              "executing",
		"active_priority_plans":        2,
		"executing_agent_tasks":        3,
		"agent_instructions_total":     6,
		"agent_instructions_by_status": map[string]any{"proposed": 2, "approved": 4},
		"persona_skill_bound":          map[string]any{"bound": 4, "unbound": 2, "total": 6},
		"cap_orchestrator_job":         map[string]any{"id": "SCH-cap-orchestrator", "present": true},
	}

	// Tab 4 Data: PM & Process
	m.MissionTitle = "Build Resilient Operating System"
	m.VisionTitle = "Universal Agent Knowledge Layer"
	m.Goals = []PMGoalRow{
		{ID: "G-1", Title: "Zero Failure Convergence", Status: "active", Metric: "failures", Target: "0"},
	}
	m.PriorityPlans = []PMPlanRow{
		{ID: "PRI-TPM-CONV", Title: "Swarm Convergence Loop", Status: "in_progress", Workstreams: []string{"core", "pm"}, BLICount: 3},
	}
	m.Workstreams = []PMWorkstreamRow{
		{ID: "WKS-PM", Title: "Program Management & Process", Status: "active"},
	}
	m.BacklogSummary = PMBacklogSummary{
		Total:      12,
		Draft:      2,
		Planned:    3,
		InProgress: 4,
		Blocked:    1,
		Done:       2,
		Claimed:    4,
		Unclaimed:  8,
	}
	m.RecentBacklog = []PMBacklogRow{
		{ID: "BLI-001", Title: "Implement TUI 6-Tab View", Status: "in_progress", Priority: "P0", ClaimedBy: "agent-alpha", PlanRef: "PRI-TPM-CONV"},
	}
	m.Blockers = []PMBlockerRow{
		{ID: "BLK-01", Title: "CAS Gate Index Contention", Severity: "high", Status: "active", Impact: "delays commits"},
	}
	m.TechnicalDebt = []PMDebtRow{
		{ID: "DEBT-01", Title: "Direct os calls in tests", Category: "hygiene", Status: "identified", Priority: "medium"},
	}

	// Tab 5 Data: Metrics
	m.Hygiene = ResourceHygieneRow{
		ProcessObjectCount: 350,
		KindCount:          24,
		StreamFileCount:    48,
		ActiveStreams:      4,
	}
	m.CommandMetrics = []CommandMetricRow{
		{ID: "CM-01", CommandName: "zqk ui", ExecCount: 14, AvgDuration: "42ms", LastRunAt: "2026-09-22 22:00:00", Status: "success"},
	}
	m.SchedulerHealth = []SchedulerHealthRow{
		{ID: "SCH-HEALTH-01", HeartbeatAt: "2026-09-22 22:00:00", Status: "healthy", Executions: 120, Failures: 0},
	}
	m.LockMetrics = []FileLockMetricRow{
		{ID: "FLM-01", TargetKind: "change_journal_entry", Contention: 0, Duration: "1.2ms", Status: "healthy"},
	}

	// Tab 6 Data: Scheduler
	m.SchedulerJobs = []SchedulerJobRow{
		{
			ID:        "SCH-autofix-run",
			Schedule:  "@every 5m",
			LastRunAt: "2026-09-22 21:00:00",
			NextRunAt: "2026-09-22 21:05:00",
			Status:    "active",
		},
	}

	// Render Tab 1: State
	m.ActiveTab = TabState
	m.DynamicMessage = "Test dynamic message line broadcast"
	out1 := Render(m)
	assert.Contains(t, out1, "MISSION CONTROL CONSOLE")
	assert.Contains(t, out1, "1: ⚡ State")
	assert.Contains(t, out1, "MESSAGE:")
	assert.Contains(t, out1, "Test dynamic message line broadcast")
	assert.Contains(t, out1, "BLI-001")
	assert.Contains(t, out1, "AUTO-SCROLL: ON")

	// Render Tab 2: Audit
	m.ActiveTab = TabAudit
	out2 := Render(m)
	assert.Contains(t, out2, "audit_event")
	assert.Contains(t, out2, "ACC-SYSTEM")
	assert.Contains(t, out2, "command_spec:zqk-state-")
	assert.Contains(t, out2, "zqk state stream")

	// Render Tab 3: Swarm
	m.ActiveTab = TabSwarm
	out3 := Render(m)
	assert.Contains(t, out3, "Multi-Agent Swarm Orchestration")
	assert.Contains(t, out3, "SCH-cap-orchestrator")
	assert.Contains(t, out3, "executing")

	// Render Tab 4: PM & Process
	m.ActiveTab = TabPM
	out4 := Render(m)
	assert.Contains(t, out4, "PM & Process Administration")
	assert.Contains(t, out4, "Build Resilient Operating System")
	assert.Contains(t, out4, "PRI-TPM-CONV")
	assert.Contains(t, out4, "BLI Pipeline (12)")
	assert.Contains(t, out4, "BLK-01")
	assert.Contains(t, out4, "DEBT-01")

	// Render Tab 5: Metrics
	m.ActiveTab = TabMetrics
	out5 := Render(m)
	assert.Contains(t, out5, "Knowledge Kernel Telemetry & Resource Hygiene")
	assert.Contains(t, out5, "350 process objects")
	assert.Contains(t, out5, "zqk ui")
	assert.Contains(t, out5, "SCH-HEALTH-01")

	// Render Tab 6: Scheduler
	m.ActiveTab = TabScheduler
	out6 := Render(m)
	assert.Contains(t, out6, "Autonomous Scheduler")
	assert.Contains(t, out6, "SCH-autofix-run")
	assert.Contains(t, out6, "@every 5m")

	// Tab 7 Data: QA & Lineage
	m.QASummary = QASummaryRow{
		TotalTestCases:    5,
		InFlightCount:     2,
		RegressionCount:   3,
		SatisfiedCriteria: 10,
		TotalCriteria:     12,
		IntactChains:      5,
		UnboundCriteria:   0,
		DoDCompliant:      true,
	}
	m.TestCases = []*test.TestCaseModel{
		{
			ID:                 "TC-CORE-001",
			Title:              "CAS Content Deduplication Integrity",
			Status:             "active",
			CompletedCriteria:  2,
			TotalCriteria:      2,
			RemainingOpenCount: 0,
			Lineage: &test.LineageChain{
				RootObject: &test.LineageNode{ID: "GOAL-01", Kind: "goal", Status: "active", Title: "Zero Data Corruption"},
				IsIntact:   true,
			},
		},
	}

	// Render Tab 7: QA
	m.ActiveTab = TabQA
	out7 := Render(m)
	assert.Contains(t, out7, "QA, Verification & Lineage Traceability")
	assert.Contains(t, out7, "TC-CORE-001")
	assert.Contains(t, out7, "CAS Content Deduplication Integrity")
	assert.Contains(t, out7, "100% Intact")
	assert.Contains(t, out7, "INTACT")
}

func TestRender_DrillDownModal(t *testing.T) {
	m := NewUIModel("", "qa")
	m.Width = 100
	m.Height = 30
	m.ActiveTab = TabQA

	m.TestCases = []*test.TestCaseModel{
		{
			ID:                 "TC-UI-007",
			Title:              "Mission Control TUI Interactive Drill-Down",
			Status:             "active",
			PathOrID:           "cmd/zqk/ui/views.go",
			Scope:              "ui",
			Category:           "tui",
			CompletedCriteria:  1,
			TotalCriteria:      1,
			RemainingOpenCount: 0,
			Lineage: &test.LineageChain{
				RootObject: &test.LineageNode{ID: "GOAL-01", Kind: "goal", Status: "active", Title: "World-Class TUI Experience"},
				IsIntact:   true,
			},
		},
	}

	// Verify Enter triggers drill-down modal
	m.SelectedIndex = 0
	handleInput(m, []byte{KeyEnter})
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "TC-UI-007", m.DetailModal.ID)

	modalOut := Render(m)
	assert.Contains(t, modalOut, "DETAILED RECORD INSPECTION")
	assert.Contains(t, modalOut, "TC-UI-007")
	assert.Contains(t, modalOut, "Mission Control TUI Interactive Drill-Down")
	assert.Contains(t, modalOut, "TRACEABILITY & LINEAGE CHAIN")

	// Verify Esc closes modal without quitting TUI
	exit := handleInput(m, []byte{KeyEsc})
	assert.False(t, exit)
	assert.Nil(t, m.DetailModal)
}

func TestRender_HealthTab_ActionCenter(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "health")
	m.Width = 100
	m.Height = 35
	assert.Equal(t, TabHealth, m.ActiveTab)

	// Populate health summary and violations
	m.HealthSummary = HealthSummary{
		OverallStatus:    "ATTENTION",
		CheckFreshness:   "FRESH (2m ago)",
		TotalViolations:  2,
		Tier1Count:       0,
		Tier2Count:       1,
		Tier3Count:       1,
		AutoFixableCount: 1,
		StaleLocksCount:  0,
		StorageFiles:     14800,
		StorageSizeStr:   "142MiB",
		OpenFileDesc:     10,
		MaxFileDesc:      245760,
	}

	m.ActionItems = []ActionCenterItem{
		{Key: "c", Name: "Quick Cache Check", Description: "Trigger non-blocking validation scan via scheduler", JobID: "SCH-cache-prewarm"},
		{Key: "a", Name: "Auto-Fix Batch", Description: "Execute batch remediation of auto-fixable issues", JobID: "SCH-autofix-run"},
	}

	m.HealthViolations = []HealthViolationRow{
		{
			Tier:        2,
			Severity:    "WARN",
			Kind:        "backlog_item",
			ObjectID:    "BLI-042",
			Category:    "reference",
			Message:     "Missing priority_plan_ref link",
			AutoFixable: true,
			Path:        ".zqk/process/backlog_items/bli_042.yaml",
		},
		{
			Tier:        3,
			Severity:    "NOTICE",
			Kind:        "scheduler_job",
			ObjectID:    "SCH-evag",
			Category:    "outstanding_validation",
			Message:     "Validation pending (outstanding)",
			AutoFixable: false,
			Path:        ".zqk/process/scheduler_jobs/sch_evag.yaml",
		},
	}

	out := Render(m)
	assert.Contains(t, out, "Kernel Integrity Radar & System Health")
	assert.Contains(t, out, "ACTION CENTER")
	assert.Contains(t, out, "[C]")
	assert.Contains(t, out, "Quick Cache Check")
	assert.Contains(t, out, "BLI-042")
	assert.Contains(t, out, "Missing priority_plan_ref link")
	assert.Contains(t, out, "SCH-evag")

	// Test triggering an Action Center key ('c')
	handleInput(m, []byte{'c'})
	assert.Contains(t, m.DynamicMessage, "Enqueued [Quick Cache Check]")

	// Test drill-down modal on selected violation
	m.SelectedIndex = 0
	handleInput(m, []byte{KeyEnter})
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "BLI-042", m.DetailModal.ID)

	modalOut := Render(m)
	assert.Contains(t, modalOut, "DETAILED RECORD INSPECTION — BLI-042")
	assert.Contains(t, modalOut, "Missing priority_plan_ref link")
	assert.Contains(t, modalOut, "Remediation")

	// Close modal
	handleInput(m, []byte{KeyEsc})
	assert.Nil(t, m.DetailModal)
}

func TestUIQuirk1_ActionCenterKeyNoCollisionWithVimNav(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "health")
	m.RefreshHealth()

	// Verify WAL maintenance action is mapped to 'm' (not 'k')
	var walAction *ActionCenterItem
	for i := range m.ActionItems {
		if strings.Contains(strings.ToLower(m.ActionItems[i].Name), "wal") {
			walAction = &m.ActionItems[i]
			break
		}
	}
	require.NotNil(t, walAction)
	assert.Equal(t, "m", walAction.Key, "WAL maintenance must be mapped to 'm' to avoid vim nav collision")

	// Verify pressing 'k' does not trigger WAL action
	m.SelectedIndex = 1
	handleInput(m, []byte{'k'})
	assert.False(t, walAction.IsTriggered, "'k' must not trigger WAL maintenance")
	assert.Equal(t, 0, m.SelectedIndex, "'k' must navigate cursor up")

	// Verify pressing 'm' triggers WAL maintenance
	handleInput(m, []byte{'m'})
	assert.True(t, walAction.IsTriggered, "'m' must trigger WAL maintenance")
	assert.Equal(t, "enqueued", walAction.Status)
}

func TestUIQuirk2_ActionStatusProgressionBadges(t *testing.T) {
	m := NewUIModel("", "health")
	m.Width = 120
	m.Height = 30

	m.ActionItems = []ActionCenterItem{
		{Key: "c", Name: "Action One", Description: "Test 1", JobID: "SCH-1", IsTriggered: true, Status: "enqueued"},
		{Key: "a", Name: "Action Two", Description: "Test 2", JobID: "SCH-2", IsTriggered: true, Status: "processing"},
		{Key: "w", Name: "Action Three", Description: "Test 3", JobID: "SCH-3", IsTriggered: true, Status: "completed"},
		{Key: "d", Name: "Action Four", Description: "Test 4", JobID: "SCH-4", IsTriggered: true, Status: "failed"},
	}

	out := Render(m)
	assert.Contains(t, out, "ENQUEUED")
	assert.Contains(t, out, "PROCESSING")
	assert.Contains(t, out, "PROCESSED")
	assert.Contains(t, out, "FAILED")
}

func TestUIQuirk3_IconSpacingAndSmooshing(t *testing.T) {
	// Wide icons must be detected and padded with 2 spaces
	assert.True(t, tds.IsWideIcon("⚙️"))
	assert.True(t, tds.IsWideIcon("⚠️"))
	assert.True(t, tds.IsWideIcon("🛡️"))
	assert.True(t, tds.IsWideIcon("⏱️"))
	assert.True(t, tds.IsWideIcon("📦"))

	assert.Equal(t, "⚙️  ", tds.IconPad("⚙️"))
	assert.Equal(t, "⚠️  ", tds.IconPad("⚠️"))

	// Compact symbols get 1 space
	assert.False(t, tds.IsWideIcon("✓"))
	assert.False(t, tds.IsWideIcon("✗"))
	assert.Equal(t, "✓ ", tds.IconPad("✓"))

	// tds.Badge with WARN must include 2 spaces after warning triangle
	warnBadge := tds.Badge("WARN")
	assert.Contains(t, warnBadge, "⚠️  WARN")
}

func TestUIQuirk4_RowCursorVisibleAcrossAllTabs(t *testing.T) {
	m := NewUIModel("", "pm")
	m.Width = 100
	m.Height = 35

	// Tab 4: PM
	m.RecentBacklog = []PMBacklogRow{
		{ID: "BLI-101", Title: "Task 1", Status: "planned", Priority: "P1"},
		{ID: "BLI-102", Title: "Task 2", Status: "in_progress", Priority: "P0"},
	}
	m.TechnicalDebt = []PMDebtRow{
		{ID: "DEBT-101", Title: "Debt 1", Priority: "high", Category: "security", Status: "active"},
	}

	m.SelectedIndex = 0
	outPM := Render(m)
	assert.Contains(t, outPM, "> BLI-101")

	// Tab 5: Metrics
	m.ActiveTab = TabMetrics
	m.CommandMetrics = []CommandMetricRow{
		{CommandName: "zqk test run", ExecCount: 12, AvgDuration: "120ms", LastRunAt: "10:00", Status: "pass"},
	}
	m.SelectedIndex = 0
	outMetrics := Render(m)
	assert.Contains(t, outMetrics, "> zqk test run")

	// Tab 6: Scheduler
	m.ActiveTab = TabScheduler
	m.SchedulerJobs = []SchedulerJobRow{
		{ID: "SCH-job-01", Schedule: "*/5 * * * *", LastRunAt: "10:00", NextRunAt: "10:05", Status: "active"},
	}
	m.SelectedIndex = 0
	outSched := Render(m)
	assert.Contains(t, outSched, "> SCH-job-01")

	// Tab 7: QA
	m.ActiveTab = TabQA
	m.TestCases = []*test.TestCaseModel{
		{ID: "TC-001", Title: "Unit Test", Status: "complete", TotalCriteria: 1, CompletedCriteria: 1},
	}
	m.SelectedIndex = 0
	outQA := Render(m)
	assert.Contains(t, outQA, "> TC-001")
}

func TestUIQuirk5_PMTabSyncAndTechDebtBounds(t *testing.T) {
	m := NewUIModel("", "pm")
	m.Width = 100
	m.Height = 35

	// Populate backlog items with complete and originated statuses
	blis := []PMBacklogRow{
		{ID: "BLI-001", Title: "Originated BLI", Status: "originated", Priority: "P1", ClaimedBy: "agent-1"},
		{ID: "BLI-002", Title: "Completed BLI", Status: "complete", Priority: "P0", ClaimedBy: "agent-2"},
	}
	m.RecentBacklog = blis
	m.TechnicalDebt = []PMDebtRow{
		{ID: "DEBT-501", Title: "Resolve WAL tail latency", Priority: "high", Category: "storage", Status: "active"},
	}

	// 1. Row count must include both BLIs and Tech Debt
	assert.Equal(t, 3, m.GetCurrentRowCount())

	// 2. Select index pointing to Technical Debt item (index 2)
	m.SelectedIndex = 2
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, objects.KindTechnicalDebt, m.DetailModal.Kind)
	assert.Equal(t, "DEBT-501", m.DetailModal.ID)
	assert.Equal(t, "Resolve WAL tail latency", m.DetailModal.Title)
}

func TestUI_MessageBannerSpacingAndEmphasis(t *testing.T) {
	m := NewUIModel("", "state")
	m.Width = 100
	m.Height = 30

	m.DynamicMessage = "✓ Job executed successfully"
	out := Render(m)
	assert.Contains(t, out, "🔔 MESSAGE:")
	assert.Contains(t, out, "✓ Job executed successfully")

	m.DynamicMessage = "⚡ Enqueued [SCH-001] to trigger queue"
	out2 := Render(m)
	assert.Contains(t, out2, "⚡ Enqueued [SCH-001]")

	m.DynamicMessage = "✗ Error during execution"
	out3 := Render(m)
	assert.Contains(t, out3, "✗ Error during execution")
}

func TestUI_SchedulerHealthTelemetryTable(t *testing.T) {
	m := NewUIModel("", "metrics")
	m.Width = 100
	m.Height = 35

	m.SchedulerHealth = []SchedulerHealthRow{
		{ID: "SH-001", HeartbeatAt: "2026-09-23 14:00:00", Status: "active", Executions: 42, Failures: 0},
		{ID: "SH-002", HeartbeatAt: "2026-09-23 14:05:00", Status: "degraded", Executions: 10, Failures: 2},
	}

	out := Render(m)
	// Must contain structured table headers, not bullet points
	assert.Contains(t, out, "SCHEDULER HEALTH TELEMETRY")
	assert.Contains(t, out, "HEALTH RECORD")
	assert.Contains(t, out, "HEARTBEAT")
	assert.Contains(t, out, "EXECUTIONS")
	assert.Contains(t, out, "FAILURES")
	assert.Contains(t, out, "SH-001")
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "SH-002")
	assert.NotContains(t, out, "• SH-001 Status:")
}

func TestUI_SchedulerJobScheduleAndTrigger(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewUIModel(tmpDir, "sched")
	m.Width = 100
	m.Height = 30

	m.SchedulerJobs = []SchedulerJobRow{
		{
			ID:            "SCH-maintenance-001",
			Title:         "System Maintenance WAL Cycle",
			Description:   "Executes periodic WAL cleanup and retention",
			Category:      "maintenance",
			JobType:       "run_wrapper",
			TriggerType:   "timer",
			ExecutionMode: "exclusive",
			MaxRuntimeSec: 600,
			Schedule:      "*/15 * * * *",
			LastRunAt:     "2026-09-23 13:45:00",
			NextRunAt:     "2026-09-23 14:00:00",
			Status:        "active",
			Command:       "/bin/sh",
			CommandArgs:   []string{"-c", "zqk system wal prune"},
		},
	}

	out := Render(m)
	assert.Contains(t, out, "SCH-maintenance-001")
	assert.Contains(t, out, "*/15 * * * *")

	// Test drill-down detail modal
	m.SelectedIndex = 0
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "SCH-maintenance-001", m.DetailModal.ID)
	assert.Equal(t, "System Maintenance WAL Cycle", m.DetailModal.Title)
	assert.Equal(t, "Executes periodic WAL cleanup and retention", m.DetailModal.Summary)

	modalRender := Render(m)
	assert.Contains(t, modalRender, "DETAILED RECORD INSPECTION")
	assert.Contains(t, modalRender, "Executes periodic WAL cleanup and retention")
	assert.Contains(t, modalRender, "Cron Schedule : */15 * * * *")
	assert.Contains(t, modalRender, "Command Exec  : /bin/sh -c zqk system wal prune")
	assert.Contains(t, modalRender, "Trigger Now   : Press [t]")

	// Dismiss modal
	handleInput(m, []byte{KeyEsc})
	assert.Nil(t, m.DetailModal)

	// Trigger immediately via 't'
	handleInput(m, []byte{'t'})
	assert.Contains(t, m.DynamicMessage, "Enqueued [SCH-maintenance-001]")

	// Trigger with delay via 'd'
	handleInput(m, []byte{'d'})
	assert.Contains(t, m.DynamicMessage, "Scheduled [SCH-maintenance-001] to trigger in 10 seconds")
}

func TestUI_QAViewTruthfulDoDAndBLICoverage(t *testing.T) {
	m := NewUIModel("", "qa")
	m.Width = 100
	m.Height = 30

	// 1. Zero test cases: must NOT report 100% Intact
	m.QASummary = QASummaryRow{
		TotalTestCases:   0,
		VerifiedBLICount: 0,
		TotalBLICount:    39,
		DoDCompliant:     false,
	}
	m.TestCases = nil

	outEmpty := Render(m)
	assert.Contains(t, outEmpty, "0% (No Test Suites)")
	assert.Contains(t, outEmpty, "PENDING")
	assert.Contains(t, outEmpty, "BLI Coverage")
	assert.Contains(t, outEmpty, "0 / 39")
	assert.Contains(t, outEmpty, "No active test case objects discovered")

	// 2. Active test cases present
	m.QASummary = QASummaryRow{
		TotalTestCases:    14,
		InFlightCount:     2,
		RegressionCount:   12,
		SatisfiedCriteria: 24,
		TotalCriteria:     27,
		IntactChains:      14,
		VerifiedBLICount:  6,
		TotalBLICount:     39,
		DoDCompliant:      true,
	}
	m.TestCases = []*test.TestCaseModel{
		{
			ID:                "TST-001",
			Title:             "Kernel State Test",
			Status:            "complete",
			CompletedCriteria: 3,
			TotalCriteria:     3,
			Lineage:           &test.LineageChain{IsIntact: true},
		},
	}

	outActive := Render(m)
	assert.Contains(t, outActive, "100% Intact")
	assert.Contains(t, outActive, "14 / 14")
	assert.Contains(t, outActive, "BLI Coverage")
	assert.Contains(t, outActive, "6 / 39")
	assert.Contains(t, outActive, "TST-001")
	assert.Contains(t, outActive, "Kernel State Test")
}

func TestUI_HealthViolationDetailMessage(t *testing.T) {
	m := NewUIModel("", "health")
	m.Width = 100
	m.Height = 30

	m.HealthViolations = []HealthViolationRow{
		{
			Kind:        "backlog_item",
			ObjectID:    "BLI-TEST-999",
			Severity:    "ERROR",
			Tier:        1,
			Category:    "structural",
			Message:     "Structural schema failure: missing required field 'priority_tier' in object specification",
			AutoFixable: false,
			Path:        ".zqk/process/backlog_items/bli_999.yaml",
		},
	}

	m.SelectedIndex = 0
	m.OpenSelectedItemDetail()
	require.NotNil(t, m.DetailModal)
	assert.Equal(t, "BLI-TEST-999", m.DetailModal.ID)
	assert.Equal(t, "Structural schema failure: missing required field 'priority_tier' in object specification", m.DetailModal.Summary)

	rendered := Render(m)
	assert.Contains(t, rendered, "DETAILED RECORD INSPECTION — BLI-TEST-999")
	assert.Contains(t, rendered, "DETAILED DIAGNOSTIC MESSAGE / SUMMARY:")
	assert.Contains(t, rendered, "Structural schema failure: missing required field 'priority_tier' in object specification")
	assert.Contains(t, rendered, "Full Message  : Structural schema failure: missing required field 'priority_tier'")
}

func TestUI_UniversalMessageLine_AllTabs(t *testing.T) {
	// BLI-OBJECT-INSPECT-MSG-001: Line 6 universal dynamic message notification pipeline across all tabs
	tabs := []int{TabState, TabAudit, TabSwarm, TabPM, TabMetrics, TabScheduler, TabQA, TabHealth}
	for _, tab := range tabs {
		m := NewUIModel("", "state")
		m.ActiveTab = tab
		m.Width = 100
		m.Height = 30
		m.DynamicMessage = "⚡ Universal Notification Broadcast Event"

		out := Render(m)
		assert.Contains(t, out, "🔔 MESSAGE:", "tab %d missing message prefix", tab)
		assert.Contains(t, out, "Universal Notification Broadcast Event", "tab %d missing dynamic message content", tab)
		assert.Contains(t, out, "MISSION CONTROL CONSOLE", "tab %d missing top console header", tab)
	}
}

func TestUI_HeaderContinuity_NoLineWrap(t *testing.T) {
	// BLI-OBJECT-INSPECT-MSG-001: Header and tabs must never exceed terminal width (no wrapping)
	widths := []int{75, 80, 90, 100, 120, 140}
	for _, w := range widths {
		m := NewUIModel("", "state")
		m.Width = w
		m.Height = 30
		m.DynamicMessage = "A very long status message that would definitely wrap around narrow terminals if not properly truncated by the dynamic message line pipeline"

		out := Render(m)
		lines := strings.Split(out, "\n")
		// Header lines are top 7 lines
		require.GreaterOrEqual(t, len(lines), 7)
		for idx := 0; idx < 7; idx++ {
			vw := tds.VisibleWidth(lines[idx])
			assert.LessOrEqual(t, vw, w, "header line %d exceeds width %d: actual width %d", idx, w, vw)
		}
	}
}

func TestUI_FooterContinuity_NoLineWrap(t *testing.T) {
	// BLI-OBJECT-INSPECT-MSG-001: Footer lines must never exceed terminal width (no wrapping)
	widths := []int{75, 80, 90, 100, 120, 140}
	for _, w := range widths {
		m := NewUIModel("", "sched")
		m.Width = w
		m.Height = 30

		out := Render(m)
		lines := strings.Split(out, "\n")
		require.GreaterOrEqual(t, len(lines), 3)
		// Last 3 lines are footer
		for i := len(lines) - 3; i < len(lines); i++ {
			vw := tds.VisibleWidth(lines[i])
			assert.LessOrEqual(t, vw, w, "footer line %d exceeds width %d: actual width %d", i, w, vw)
		}
	}
}

func TestUI_RefreshDynamicMessage_FeedIntegration(t *testing.T) {
	// BLI-OBJECT-INSPECT-MSG-001: Dynamic message hydrates from agent chat feed and mutations
	tmpDir := t.TempDir()
	logDir := tmpDir + "/.zqk/logs/ide-hooks"
	require.NoError(t, os.MkdirAll(logDir, 0755))
	chatFile := logDir + "/agent_chat_channel.jsonl"
	chatEntry := `{"type":"kernel_change","operation":"update","kind":"backlog_item","object_id":"BLI-001","message":"Object backlog_item updated: BLI-001 (backlog_item)"}` + "\n"
	require.NoError(t, os.WriteFile(chatFile, []byte(chatEntry), 0644))

	m := NewUIModel(tmpDir, "state")
	m.RefreshDynamicMessage()
	assert.Contains(t, m.DynamicMessage, "BLI-001")
	assert.Contains(t, m.DynamicMessage, "⚡")
}

func TestUI_VimTopBottomNavigation_gG_Toggling(t *testing.T) {
	m := NewUIModel("", "pm")
	m.RecentBacklog = []PMBacklogRow{
		{ID: "BLI-001", Title: "Task 1", Status: "planned"},
		{ID: "BLI-002", Title: "Task 2", Status: "in_progress"},
		{ID: "BLI-003", Title: "Task 3", Status: "testing"},
		{ID: "BLI-004", Title: "Task 4", Status: "done"},
		{ID: "BLI-005", Title: "Task 5", Status: "complete"},
	}
	assert.Equal(t, 5, m.GetCurrentRowCount())

	// Start at 0, jump to bottom with 'G' (big gee)
	m.SelectedIndex = 0
	handleInput(m, []byte{'G'})
	assert.Equal(t, 4, m.SelectedIndex, "G should jump to bottom")

	// Jump to top with 'g' (little gee)
	handleInput(m, []byte{'g'})
	assert.Equal(t, 0, m.SelectedIndex, "g should jump to top")

	// Toggle g -> G when already at top
	handleInput(m, []byte{'g'})
	assert.Equal(t, 4, m.SelectedIndex, "g when at top should toggle to bottom (g->G)")

	// Toggle G -> g when already at bottom
	handleInput(m, []byte{'G'})
	assert.Equal(t, 0, m.SelectedIndex, "G when at bottom should toggle to top (G->g)")

	// Home and End ANSI sequences
	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, 'F'}) // End
	assert.Equal(t, 4, m.SelectedIndex, "End key should jump to bottom")
	handleInput(m, []byte{CSIPrefixEsc, CSIPrefixBracket, 'H'}) // Home
	assert.Equal(t, 0, m.SelectedIndex, "Home key should jump to top")
}

func TestUI_InlineSearch_FilteringAndUnwinding(t *testing.T) {
	m := NewUIModel("", "pm")
	m.RecentBacklog = []PMBacklogRow{
		{ID: "BLI-001", Title: "Fix WAL deadlock contention", Status: "planned"},
		{ID: "BLI-002", Title: "Implement object inspect CLI", Status: "in_progress"},
		{ID: "BLI-003", Title: "Optimize CAS storage hygiene", Status: "testing"},
		{ID: "BLI-004", Title: "TDS component color fixes", Status: "done"},
	}
	m.TechnicalDebt = []PMDebtRow{
		{ID: "DEBT-001", Title: "Legacy CAS inspection wrappers", Category: "storage"},
		{ID: "DEBT-002", Title: "Vim navigation keybinding", Category: "ui"},
	}
	assert.Equal(t, 6, m.GetCurrentRowCount())

	// 1. Press '/' to initiate search
	handleInput(m, []byte{'/'})
	assert.True(t, m.IsSearching, "IsSearching should be true after '/'")

	// 2. Type 'c', 'a', 's'
	handleInput(m, []byte{'c'})
	handleInput(m, []byte{'a'})
	handleInput(m, []byte{'s'})
	assert.Equal(t, "cas", m.SearchBuffer)
	assert.Equal(t, "cas", m.SearchQuery)

	// In live filtering, only BLI-003 and DEBT-001 match 'cas'
	assert.Equal(t, 2, m.GetCurrentRowCount(), "Should match 1 BLI and 1 Debt item")
	visibleBLI := m.GetVisibleBacklog()
	require.Len(t, visibleBLI, 1)
	assert.Equal(t, "BLI-003", visibleBLI[0].ID)
	visibleDebt := m.GetVisibleTechnicalDebt()
	require.Len(t, visibleDebt, 1)
	assert.Equal(t, "DEBT-001", visibleDebt[0].ID)

	// 3. Test Backspace
	handleInput(m, []byte{KeyBackspace})
	assert.Equal(t, "ca", m.SearchQuery)

	// Type 's' back
	handleInput(m, []byte{'s'})
	assert.Equal(t, "cas", m.SearchQuery)

	// 4. Confirm search with Enter
	handleInput(m, []byte{KeyEnter})
	assert.False(t, m.IsSearching, "IsSearching should be false after Enter")
	assert.Equal(t, "cas", m.SearchQuery, "SearchQuery should remain locked")

	// 5. Test n / N match cycling
	assert.Equal(t, 0, m.SelectedIndex)
	handleInput(m, []byte{'n'})
	assert.Equal(t, 1, m.SelectedIndex, "n should cycle to second match")
	handleInput(m, []byte{'n'})
	assert.Equal(t, 0, m.SelectedIndex, "n should wrap back to first match")
	handleInput(m, []byte{'N'})
	assert.Equal(t, 1, m.SelectedIndex, "N should cycle backward")

	// 6. Test Header Rendering with search filter banner
	m.Width = 100
	m.Height = 30
	rendered := Render(m)
	assert.Contains(t, rendered, "SEARCH:")
	assert.Contains(t, rendered, "cas")
	assert.Contains(t, rendered, "2 matches")

	// 7. Test Drill-Down modal inspection on filtered item
	m.SelectedIndex = 0
	handleInput(m, []byte{KeyEnter})
	require.NotNil(t, m.DetailModal, "Enter on filtered row should open detail modal")
	assert.Equal(t, "BLI-003", m.DetailModal.ID)

	// 8. Test Esc unwinding:
	// Esc 1: Dismiss modal, remain in search filter
	handleInput(m, []byte{KeyEsc})
	assert.Nil(t, m.DetailModal, "Esc should dismiss detail modal")
	assert.Equal(t, "cas", m.SearchQuery, "SearchQuery should still be active")

	// Esc 2: Clear search filter, restore full backlog
	handleInput(m, []byte{KeyEsc})
	assert.Equal(t, "", m.SearchQuery, "Esc should clear search filter")
	assert.Equal(t, 6, m.GetCurrentRowCount(), "All 6 items should be restored")

	// Esc 3: Exit application
	shouldExit := handleInput(m, []byte{KeyEsc})
	assert.True(t, shouldExit, "Esc with clean state should exit UI")
}



