package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/zqk-os/zqk/cmd/zqk/state"
)

var (
	cyanBold   = color.New(color.FgCyan, color.Bold).SprintFunc()
	whiteBold  = color.New(color.FgWhite, color.Bold).SprintFunc()
	yellowBold = color.New(color.FgYellow, color.Bold).SprintFunc()
	greenBold  = color.New(color.FgGreen, color.Bold).SprintFunc()
	redBold    = color.New(color.FgRed, color.Bold).SprintFunc()
	dim        = color.New(color.Faint).SprintFunc()
	reverse    = color.New(color.ReverseVideo).SprintFunc()
)

// Render formats the entire TUI screen into an ANSI string.
func Render(m *UIModel) string {
	var b strings.Builder

	// Top Navigation Header
	renderHeader(&b, m)

	// Tab-Specific Body
	switch m.ActiveTab {
	case TabSeismograph:
		renderSeismographTab(&b, m)
	case TabSwarm:
		renderSwarmTab(&b, m)
	case TabObjects:
		renderObjectsTab(&b, m)
	case TabScheduler:
		renderSchedulerTab(&b, m)
	default:
		renderSeismographTab(&b, m)
	}

	// Bottom Status & Help Bar
	renderFooter(&b, m)

	return b.String()
}

func renderHeader(b *strings.Builder, m *UIModel) {
	b.WriteString("╔═══════════════════════════════════════════════════════════════════════════════════════════════╗\n")
	b.WriteString("║                 ⚡ ZQK KNOWLEDGE KERNEL — MISSION CONTROL CONSOLE                             ║\n")
	b.WriteString("╚═══════════════════════════════════════════════════════════════════════════════════════════════╝\n")

	// Tabs Bar
	tabs := []struct {
		idx  int
		name string
	}{
		{TabSeismograph, "1: ⚡ Seismograph"},
		{TabSwarm, "2: 🤖 Swarm"},
		{TabObjects, "3: 📋 Objects"},
		{TabScheduler, "4: ⏱️ Scheduler"},
	}

	var tabStrs []string
	for _, t := range tabs {
		if t.idx == m.ActiveTab {
			tabStrs = append(tabStrs, reverse(fmt.Sprintf(" [%s] ", t.name)))
		} else {
			tabStrs = append(tabStrs, dim(fmt.Sprintf("  %s  ", t.name)))
		}
	}
	b.WriteString(strings.Join(tabStrs, " │ ") + "\n")
	b.WriteString(dim("───────────────────────────────────────────────────────────────────────────────────────────\n"))
}

func renderSeismographTab(b *strings.Builder, m *UIModel) {
	// Calculate kind counts from recent mutations
	kindCounts := make(map[string]int)
	for _, mut := range m.Mutations {
		parts := strings.Split(mut.ObjectRef, ":")
		if len(parts) > 0 && parts[0] != "" {
			kindCounts[parts[0]]++
		}
	}

	// Sparkline
	sparkline := generateSparkline(len(m.Mutations))

	// Symbol counters
	symbolCounters := formatSymbolCounters(kindCounts)

	// Auto-scroll badge
	scrollMode := greenBold("[AUTO-SCROLL: ON]")
	if !m.AutoScroll {
		scrollMode = yellowBold(fmt.Sprintf("[PAUSED: +%d]", m.ScrollOffset))
	}

	b.WriteString(fmt.Sprintf("Status: %s | Buffer: %d events | Rate: %s | %s\n",
		greenBold("ENFORCING"), len(m.Mutations), sparkline, scrollMode))
	b.WriteString(fmt.Sprintf("Types:  %s\n", symbolCounters))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("%-10s │ %-12s │ %-32s │ %-30s\n", "TIME", "EVENT", "OBJECT REF", "SUMMARY / DIFF"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	// Determine visible lines based on terminal height
	availRows := m.Height - 12
	if availRows < 6 {
		availRows = 6
	}

	total := len(m.Mutations)
	var visible []state.JournalMutation

	if total == 0 {
		b.WriteString(dim("  [No stream events recorded yet. Perform an action to see real-time events]\n"))
	} else {
		if m.AutoScroll {
			start := total - availRows
			if start < 0 {
				start = 0
			}
			visible = m.Mutations[start:]
		} else {
			end := total - m.ScrollOffset
			if end > total {
				end = total
			}
			start := end - availRows
			if start < 0 {
				start = 0
			}
			visible = m.Mutations[start:end]
		}

		for _, mut := range visible {
			tStr := "--:--:--"
			if mut.CreatedAt > 0 {
				tStr = time.Unix(mut.CreatedAt, 0).Format("15:04:05")
			}

			badge := state.FormatEventBadge(mut.ChangeType)
			ref := mut.ObjectRef
			if len(ref) > 32 {
				ref = ref[:29] + "..."
			}

			summary := mut.DiffSummary
			if summary == "" {
				summary = mut.ChangeType
			}
			if len(summary) > 30 {
				summary = summary[:27] + "..."
			}

			b.WriteString(fmt.Sprintf("[%s] │ %-12s │ %-32s │ %s\n", tStr, badge, ref, summary))
		}
	}

	// Pad remaining rows so layout remains stable
	for i := len(visible); i < availRows; i++ {
		b.WriteString("\n")
	}
}

func renderSwarmTab(b *strings.Builder, m *UIModel) {
	b.WriteString(whiteBold("🤖 Multi-Agent Swarm Orchestration & Throughput (MMORCH)\n"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	if m.SwarmData == nil {
		b.WriteString(dim("  [Swarm status unavailable or storage initializing...]\n\n"))
		return
	}

	hint := fmt.Sprintf("%v", m.SwarmData["throughput_hint"])
	plans := fmt.Sprintf("%v", m.SwarmData["active_priority_plans"])
	tasks := fmt.Sprintf("%v", m.SwarmData["executing_agent_tasks"])
	instTotal := fmt.Sprintf("%v", m.SwarmData["agent_instructions_total"])

	b.WriteString(fmt.Sprintf("Throughput Status:       %s\n", cyanBold(hint)))
	b.WriteString(fmt.Sprintf("Active Priority Plans:   %s\n", yellowBold(plans)))
	b.WriteString(fmt.Sprintf("Executing Agent Tasks:   %s\n", greenBold(tasks)))
	b.WriteString(fmt.Sprintf("Total Instructions:      %s\n\n", whiteBold(instTotal)))

	if instByStatus, ok := m.SwarmData["agent_instructions_by_status"].(map[string]any); ok && len(instByStatus) > 0 {
		b.WriteString(whiteBold("Instruction Breakdown:\n"))
		for status, count := range instByStatus {
			b.WriteString(fmt.Sprintf("  • %-16s %v\n", status+":", count))
		}
		b.WriteString("\n")
	}

	if personaInfo, ok := m.SwarmData["persona_skill_bound"].(map[string]any); ok {
		bound := fmt.Sprintf("%v", personaInfo["bound"])
		total := fmt.Sprintf("%v", personaInfo["total"])
		unbound := fmt.Sprintf("%v", personaInfo["unbound"])
		b.WriteString(fmt.Sprintf("Agent Seating / Skills:  %s bound, %s unbound (Total: %s personas)\n",
			greenBold(bound), yellowBold(unbound), total))
	}

	if capJob, ok := m.SwarmData["cap_orchestrator_job"].(map[string]any); ok {
		id := fmt.Sprintf("%v", capJob["id"])
		present := fmt.Sprintf("%v", capJob["present"])
		b.WriteString(fmt.Sprintf("CAP Orchestrator Daemon: %s (registered: %s)\n", cyanBold(id), present))
	}
}

func renderObjectsTab(b *strings.Builder, m *UIModel) {
	b.WriteString(whiteBold("📋 Knowledge Kernel Object Inventory & Process Data Cells\n"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	if len(m.ObjectCounts) == 0 {
		b.WriteString(dim("  [No process objects discovered in .zqk/process/]\n\n"))
		return
	}

	// Sort kinds alphabetically
	kinds := make([]string, 0, len(m.ObjectCounts))
	total := 0
	for k, c := range m.ObjectCounts {
		kinds = append(kinds, k)
		total += c
	}
	sort.Strings(kinds)

	b.WriteString(fmt.Sprintf("Total Discovered Objects: %s across %s kinds\n\n",
		greenBold(fmt.Sprintf("%d", total)), whiteBold(fmt.Sprintf("%d", len(kinds)))))

	b.WriteString(fmt.Sprintf("%-28s │ %-8s │ %-45s\n", "OBJECT KIND", "COUNT", "DESCRIPTION"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, k := range kinds {
		c := m.ObjectCounts[k]
		desc := describeKind(k)
		b.WriteString(fmt.Sprintf("%-28s │ %-8d │ %s\n", k, c, dim(desc)))
	}
}

func renderSchedulerTab(b *strings.Builder, m *UIModel) {
	b.WriteString(whiteBold("⏱️ Autonomous Scheduler & Background Daemons\n"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	if len(m.SchedulerJobs) == 0 {
		b.WriteString(dim("  [No scheduler jobs registered or scheduler not initialized]\n\n"))
		return
	}

	b.WriteString(fmt.Sprintf("%-30s │ %-12s │ %-19s │ %-19s │ %s\n",
		"JOB IDENTIFIER", "SCHEDULE", "LAST RUN", "NEXT RUN", "STATUS"))
	b.WriteString("───────────────────────────────────────────────────────────────────────────────────────────\n")

	for _, job := range m.SchedulerJobs {
		stBadge := greenBold(job.Status)
		if job.Status == "failed" || job.Status == "error" {
			stBadge = redBold(job.Status)
		} else if job.Status == "paused" {
			stBadge = yellowBold(job.Status)
		}

		b.WriteString(fmt.Sprintf("%-30s │ %-12s │ %-19s │ %-19s │ %s\n",
			job.ID, job.Schedule, job.LastRunAt, job.NextRunAt, stBadge))
	}
}

func renderFooter(b *strings.Builder, m *UIModel) {
	b.WriteString(dim("───────────────────────────────────────────────────────────────────────────────────────────\n"))
	// Legend Bar
	b.WriteString(dim("Legend: ") + "⚡ Audit | 📋 Backlog | ⚙️ Scheduler | 🤖 Agent | 📦 Change | 🎯 Requirement\n")
	// Keybindings Bar
	b.WriteString(whiteBold("[Tab / 1-4]") + " Switch View  " +
		whiteBold("[↑/↓/j/k]") + " Scroll  " +
		whiteBold("[Space]") + " Pause/Resume  " +
		whiteBold("[r]") + " Refresh  " +
		whiteBold("[q/Esc]") + " Exit\n")
}

func formatSymbolCounters(counts map[string]int) string {
	mapping := []struct {
		kind   string
		symbol string
		tag    string
	}{
		{"audit_event", "⚡", "AUD"},
		{"scheduler_job", "⚙️", "SCH"},
		{"backlog_item", "📋", "BLI"},
		{"change_journal_entry", "📦", "CHA"},
		{"agent_task", "🤖", "ATK"},
		{"agent_instruction", "📨", "AGI"},
		{"requirement", "🎯", "REQ"},
		{"test_case", "🧪", "TCA"},
	}

	var parts []string
	for _, m := range mapping {
		if c, ok := counts[m.kind]; ok && c > 0 {
			parts = append(parts, fmt.Sprintf("%s %s: %d", m.symbol, m.tag, c))
		}
	}

	if len(parts) == 0 {
		return dim("none")
	}
	return strings.Join(parts, "  │  ")
}

func generateSparkline(count int) string {
	bars := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	if count == 0 {
		return "[        ]"
	}
	var b strings.Builder
	b.WriteRune('[')
	for i := 0; i < 8; i++ {
		idx := (count * (i + 1)) / 8
		if idx >= len(bars) {
			idx = len(bars) - 1
		}
		b.WriteRune(bars[idx])
	}
	b.WriteRune(']')
	return b.String()
}

func describeKind(k string) string {
	switch k {
	case "backlog_item", "backlog_items":
		return "Executable work unit (BLI)"
	case "priority_plan", "priority_plans":
		return "Sprint/milestone plan boundary (PRI)"
	case "requirement", "requirements":
		return "Functional or technical requirement (REQ)"
	case "criteria":
		return "Acceptance test criteria (CRT)"
	case "test_case", "test_cases":
		return "Executable test case chain (TCA)"
	case "workstream", "workstreams":
		return "Domain workstream lane (WKS)"
	case "scheduler_job", "scheduler_jobs":
		return "Automated maintenance daemon job (SCH)"
	case "agent_task", "agent_tasks":
		return "Autonomous swarm task lease (ATK)"
	case "agent_instruction", "agent_instructions":
		return "Peer correspondence instruction (AGI)"
	case "audit_event", "audit_events":
		return "Immutable operational audit record (AUD)"
	case "change_journal_entry", "change_journal_entries":
		return "Delta mutation log record (CHA)"
	case "milestone", "milestones":
		return "Program milestone target (MIL)"
	case "policy", "policies":
		return "Repository governance rule (POL)"
	case "persona", "personas":
		return "Agent host seating persona (PER)"
	case "agent_skill", "agent_skills":
		return "Agent persona capability pack"
	case "glossary_term", "glossary_terms":
		return "Knowledge domain ontology term"
	case "qa_success":
		return "Automated test gate success record"
	case "risk_blocker", "risk_blockers":
		return "Active risk and operational blocker"
	case "goal", "goals":
		return "Strategic program goal"
	case "mission", "missions":
		return "Core system mission definition"
	case "vision", "visions":
		return "Product vision and strategic intent"
	default:
		return "Knowledge kernel data cell"
	}
}
