package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/state"
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

func TestUIModel_NavigationAndScrolling(t *testing.T) {
	m := NewUIModel("", "seismograph")
	assert.Equal(t, TabSeismograph, m.ActiveTab)
	assert.True(t, m.AutoScroll)
	assert.Equal(t, 0, m.ScrollOffset)

	// Tab cycle forward
	exit := handleInput(m, []byte{'\t'})
	assert.False(t, exit)
	assert.Equal(t, TabSwarm, m.ActiveTab)

	// Direct tab selection
	handleInput(m, []byte{'3'})
	assert.Equal(t, TabObjects, m.ActiveTab)

	handleInput(m, []byte{'4'})
	assert.Equal(t, TabScheduler, m.ActiveTab)

	handleInput(m, []byte{'1'})
	assert.Equal(t, TabSeismograph, m.ActiveTab)

	// Test scroll controls
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

	// Toggle pause
	handleInput(m, []byte{' '})
	assert.False(t, m.AutoScroll)

	handleInput(m, []byte{' '})
	assert.True(t, m.AutoScroll)

	// Exit keys
	assert.True(t, handleInput(m, []byte{'q'}))
	assert.True(t, handleInput(m, []byte{'Q'}))
	assert.True(t, handleInput(m, []byte{27})) // Esc
	assert.True(t, handleInput(m, []byte{3}))  // Ctrl+C
}

func TestRender_AllTabs(t *testing.T) {
	m := NewUIModel("", "seismograph")
	m.Width = 100
	m.Height = 30

	m.Mutations = []state.JournalMutation{
		{
			ID:          "AUD-100",
			ChangeType:  "system_config_change",
			ObjectRef:   "audit_event:AUD-100",
			DiffSummary: "Queue state changed",
			CreatedAt:   time.Now().Unix(),
		},
		{
			ID:          "SCH-101",
			ChangeType:  "update",
			ObjectRef:   "scheduler_job:SCH-autofix-run",
			DiffSummary: "next_run_at updated",
			CreatedAt:   time.Now().Unix(),
		},
	}

	m.ObjectCounts = map[string]int{
		"backlog_item": 5,
		"scheduler_job": 12,
		"audit_event": 42,
	}

	m.SwarmData = map[string]any{
		"throughput_hint":              "executing",
		"active_priority_plans":        1,
		"executing_agent_tasks":        2,
		"agent_instructions_total":     4,
		"agent_instructions_by_status": map[string]any{"proposed": 2, "approved": 2},
		"persona_skill_bound":          map[string]any{"bound": 3, "unbound": 6, "total": 9},
		"cap_orchestrator_job":         map[string]any{"id": "SCH-cap-orchestrator", "present": true},
	}

	m.SchedulerJobs = []SchedulerJobRow{
		{
			ID:        "SCH-autofix-run",
			Schedule:  "@every 5m",
			LastRunAt: "2026-09-22 21:00:00",
			NextRunAt: "2026-09-22 21:05:00",
			Status:    "active",
		},
	}

	// Tab 1: Seismograph
	m.ActiveTab = TabSeismograph
	out1 := Render(m)
	assert.Contains(t, out1, "MISSION CONTROL CONSOLE")
	assert.Contains(t, out1, "AUD-100")
	assert.Contains(t, out1, "SCH-autofix-run")
	assert.Contains(t, out1, "AUTO-SCROLL: ON")

	// Tab 2: Swarm
	m.ActiveTab = TabSwarm
	out2 := Render(m)
	assert.Contains(t, out2, "Multi-Agent Swarm Orchestration")
	assert.Contains(t, out2, "SCH-cap-orchestrator")
	assert.Contains(t, out2, "executing")

	// Tab 3: Objects
	m.ActiveTab = TabObjects
	out3 := Render(m)
	assert.Contains(t, out3, "Knowledge Kernel Object Inventory")
	assert.Contains(t, out3, "backlog_item")
	assert.Contains(t, out3, "Executable work unit (BLI)")

	// Tab 4: Scheduler
	m.ActiveTab = TabScheduler
	out4 := Render(m)
	assert.Contains(t, out4, "Autonomous Scheduler")
	assert.Contains(t, out4, "SCH-autofix-run")
	assert.Contains(t, out4, "@every 5m")
}
