package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/test"
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

