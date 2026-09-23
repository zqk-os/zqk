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
	case TabState:
		renderStateTab(&b, m)
	case TabAudit:
		renderAuditTab(&b, m)
	case TabSwarm:
		renderSwarmTab(&b, m)
	case TabPM:
		renderPMTab(&b, m)
	case TabMetrics:
		renderMetricsTab(&b, m)
	case TabScheduler:
		renderSchedulerTab(&b, m)
	default:
		renderStateTab(&b, m)
	}

	// Bottom Status & Help Bar
	renderFooter(&b, m)

	return b.String()
}

func renderHeader(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	b.WriteString("╔" + strings.Repeat("═", w-2) + "╗\n")
	title := "⚡ ZQK KNOWLEDGE KERNEL — MISSION CONTROL CONSOLE"
	innerW := w - 2
	titleRunes := []rune(title)
	if len(titleRunes) > innerW-2 {
		title = string(titleRunes[:innerW-5]) + "..."
		titleRunes = []rune(title)
	}
	pad := innerW - len(titleRunes)
	if pad < 0 {
		pad = 0
	}
	leftPad := pad / 2
	rightPad := pad - leftPad
	b.WriteString("║" + strings.Repeat(" ", leftPad) + cyanBold(title) + strings.Repeat(" ", rightPad) + "║\n")
	b.WriteString("╚" + strings.Repeat("═", w-2) + "╝\n")

	// Tabs Bar (6 tabs)
	tabs := []struct {
		idx  int
		name string
	}{
		{TabState, "1: ⚡ State"},
		{TabAudit, "2: 📜 Audit"},
		{TabSwarm, "3: 🤖 Swarm"},
		{TabPM, "4: 📋 PM & Process"},
		{TabMetrics, "5: 📊 Metrics"},
		{TabScheduler, "6: ⏱️ Scheduler"},
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
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")
}

// renderStateTab renders real-time state mutations, instructions, and lifecycles (excluding high-volume audit logs).
func renderStateTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	kindCounts := make(map[string]int)
	for _, mut := range m.Mutations {
		parts := strings.Split(mut.ObjectRef, ":")
		if len(parts) > 0 && parts[0] != "" {
			kindCounts[parts[0]]++
		}
	}

	sparkline := generateSparkline(len(m.Mutations))
	symbolCounters := formatSymbolCounters(kindCounts)

	scrollMode := greenBold("[AUTO-SCROLL: ON]")
	if !m.AutoScroll {
		scrollMode = yellowBold(fmt.Sprintf("[PAUSED: +%d]", m.ScrollOffset))
	}

	b.WriteString(fmt.Sprintf("Status: %s │ Buffer: %d state mutations │ Rate: %s │ %s\n",
		greenBold("ENFORCING"), len(m.Mutations), sparkline, scrollMode))
	b.WriteString(fmt.Sprintf("Types:  %s\n", symbolCounters))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	timeW := 10
	eventW := 12
	refW := 24
	if w >= 110 {
		refW = 32
	}
	sumW := w - timeW - eventW - refW - 9
	if sumW < 14 {
		refW = 18
		sumW = w - timeW - eventW - refW - 9
	}
	if sumW < 10 {
		sumW = 10
	}

	b.WriteString(fmt.Sprintf("%-*s │ %-*s │ %-*s │ %s\n",
		timeW, "TIME", eventW, "EVENT", refW, "OBJECT REF", "SUMMARY / DIFF"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	availRows := m.Height - 12
	if availRows < 6 {
		availRows = 6
	}

	total := len(m.Mutations)
	var visible []state.JournalMutation

	if total == 0 {
		b.WriteString(dim("  [No state mutations recorded in stream buffer yet]\n"))
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
			if len(ref) > refW {
				ref = ref[:refW-3] + "..."
			}

			summary := mut.DiffSummary
			if summary == "" {
				summary = mut.ChangeType
			}
			if len(summary) > sumW {
				summary = summary[:sumW-3] + "..."
			}

			b.WriteString(fmt.Sprintf("[%s] │ %-*s │ %-*s │ %s\n",
				tStr, eventW, badge, refW, ref, summary))
		}
	}

	for i := len(visible); i < availRows; i++ {
		b.WriteString("\n")
	}
}

// renderAuditTab renders the dedicated high-volume operational audit stream.
func renderAuditTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	// Actor breakdown
	actors := make(map[string]int)
	for _, a := range m.AuditEvents {
		act := a.CreatedBy
		if act == "" {
			act = a.Actor
		}
		if act == "" {
			act = "system"
		}
		actors[act]++
	}

	var actorParts []string
	for act, cnt := range actors {
		actorParts = append(actorParts, fmt.Sprintf("%s (%d)", act, cnt))
	}
	sort.Strings(actorParts)
	actorLine := strings.Join(actorParts, ", ")
	if len(actorLine) > w-25 && len(actorLine) > 20 {
		actorLine = actorLine[:w-28] + "..."
	}

	sparkline := generateSparkline(len(m.AuditEvents))
	scrollMode := greenBold("[AUTO-SCROLL: ON]")
	if !m.AutoScroll {
		scrollMode = yellowBold(fmt.Sprintf("[PAUSED: +%d]", m.ScrollOffset))
	}

	b.WriteString(fmt.Sprintf("Stream: %s │ Buffer: %d audit events │ Rate: %s │ %s\n",
		cyanBold("audit_event"), len(m.AuditEvents), sparkline, scrollMode))
	b.WriteString(fmt.Sprintf("Actors: %s\n", actorLine))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	timeW := 10
	actorW := 16
	opW := 18
	refW := 22
	if w >= 110 {
		actorW = 18
		opW = 20
		refW = 26
	}
	detailW := w - timeW - actorW - opW - refW - 12
	if detailW < 12 {
		detailW = 12
	}

	b.WriteString(fmt.Sprintf("%-*s │ %-*s │ %-*s │ %-*s │ %s\n",
		timeW, "TIME", actorW, "ACTOR", opW, "OPERATION", refW, "OBJECT REF", "DETAILS / PAYLOAD"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	availRows := m.Height - 12
	if availRows < 6 {
		availRows = 6
	}

	total := len(m.AuditEvents)
	var visible []state.JournalMutation

	if total == 0 {
		b.WriteString(dim("  [No audit events recorded in audit stream yet]\n"))
	} else {
		if m.AutoScroll {
			start := total - availRows
			if start < 0 {
				start = 0
			}
			visible = m.AuditEvents[start:]
		} else {
			end := total - m.ScrollOffset
			if end > total {
				end = total
			}
			start := end - availRows
			if start < 0 {
				start = 0
			}
			visible = m.AuditEvents[start:end]
		}

		for _, aud := range visible {
			tStr := "--:--:--"
			if aud.CreatedAt > 0 {
				tStr = time.Unix(aud.CreatedAt, 0).Format("15:04:05")
			}

			act := aud.CreatedBy
			if act == "" {
				act = aud.Actor
			}
			if act == "" {
				act = "system"
			}
			if len(act) > actorW {
				act = act[:actorW-3] + "..."
			}

			op := aud.Operation
			if op == "" {
				op = aud.ChangeType
			}
			if len(op) > opW {
				op = op[:opW-3] + "..."
			}

			ref := aud.ObjectRef
			if len(ref) > refW {
				ref = ref[:refW-3] + "..."
			}

			detail := aud.DiffSummary
			if detail == "" {
				detail = "--"
			}
			if len(detail) > detailW {
				detail = detail[:detailW-3] + "..."
			}

			b.WriteString(fmt.Sprintf("[%s] │ %-*s │ %-*s │ %-*s │ %s\n",
				tStr, actorW, act, opW, op, refW, ref, detail))
		}
	}

	for i := len(visible); i < availRows; i++ {
		b.WriteString("\n")
	}
}

func renderSwarmTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(whiteBold("🤖 Multi-Agent Swarm Orchestration & Throughput (MMORCH)\n"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	if m.SwarmData == nil {
		b.WriteString(dim("  [Swarm status initializing or storage unavailable...]\n\n"))
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

// renderPMTab renders the dedicated PM & Process Admin dashboard (Strategic cascade, delivery pipeline, blockers).
func renderPMTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	b.WriteString(whiteBold("📋 Program & Process Management Administration (TPM / PM Cascade)\n"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	// 1. Strategic Alignment (Mission, Vision, Goals)
	mis := m.MissionTitle
	if mis == "" {
		mis = "Continuous Autonomous Development"
	}
	vis := m.VisionTitle
	if vis == "" {
		vis = "Zero-Overhead Agentic Knowledge Operating System"
	}

	b.WriteString(fmt.Sprintf("Mission: %s │ Vision: %s\n", cyanBold(mis), dim(vis)))

	if len(m.Goals) > 0 {
		var goalParts []string
		for _, g := range m.Goals {
			goalParts = append(goalParts, fmt.Sprintf("%s: %s [%s]", g.ID, g.Title, greenBold(g.Status)))
		}
		b.WriteString(fmt.Sprintf("Goals:   %s\n", strings.Join(goalParts, " │ ")))
	}
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	// 2. Delivery Pipeline Summary Box
	bs := m.BacklogSummary
	b.WriteString(fmt.Sprintf("%s │ %s: %d  %s: %d  %s: %d  %s: %d  %s: %d  %s: %d  (%s claimed / %s open)\n",
		whiteBold(fmt.Sprintf("BLI Pipeline (%d Total)", bs.Total)),
		dim("Draft"), bs.Draft,
		cyanBold("Planned"), bs.Planned,
		yellowBold("InProg"), bs.InProgress,
		redBold("Blocked"), bs.Blocked,
		greenBold("Done"), bs.Done,
		whiteBold("Approved"), bs.Approved,
		greenBold(fmt.Sprintf("%d", bs.Claimed)),
		yellowBold(fmt.Sprintf("%d", bs.Unclaimed))))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	// 3. Priority Plans & Workstreams
	if len(m.PriorityPlans) > 0 {
		b.WriteString(whiteBold("Active Priority Plans:\n"))
		for _, p := range m.PriorityPlans {
			wsStr := strings.Join(p.Workstreams, ", ")
			if wsStr == "" {
				wsStr = "core"
			}
			b.WriteString(fmt.Sprintf("  • %-16s %-32s status: %-10s (workstreams: %s, BLIs: %d)\n",
				cyanBold(p.ID), p.Title, yellowBold(p.Status), dim(wsStr), p.BLICount))
		}
		b.WriteString("\n")
	}

	// 4. Active Blockers & Risks
	if len(m.Blockers) > 0 {
		b.WriteString(redBold("Active Risks & Blockers:\n"))
		for _, blk := range m.Blockers {
			b.WriteString(fmt.Sprintf("  ⚠️  %-12s %-30s [Sev: %s | Impact: %s | Status: %s]\n",
				blk.ID, blk.Title, redBold(blk.Severity), blk.Impact, blk.Status))
		}
		b.WriteString("\n")
	}

	// 5. Technical Debt
	if len(m.TechnicalDebt) > 0 {
		b.WriteString(yellowBold("Technical Debt Items:\n"))
		for _, d := range m.TechnicalDebt {
			b.WriteString(fmt.Sprintf("  🔧 %-12s %-30s [Category: %s | Priority: %s | Status: %s]\n",
				d.ID, d.Title, d.Category, d.Priority, d.Status))
		}
		b.WriteString("\n")
	}

	// 6. Recent Backlog Items Table
	if len(m.RecentBacklog) > 0 {
		b.WriteString(whiteBold("Recent Work Units (Backlog Items):\n"))
		bliIDW := 14
		prioW := 6
		stW := 12
		claimW := 16
		titleW := w - bliIDW - prioW - stW - claimW - 14
		if titleW < 15 {
			titleW = 15
		}

		b.WriteString(fmt.Sprintf("  %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
			bliIDW, "BLI ID", prioW, "PRIO", stW, "STATUS", claimW, "CLAIMED BY", "TITLE"))
		b.WriteString("  " + dim(strings.Repeat("─", w-4)) + "\n")

		limit := 6
		if len(m.RecentBacklog) < limit {
			limit = len(m.RecentBacklog)
		}
		for i := 0; i < limit; i++ {
			item := m.RecentBacklog[i]
			stBadge := item.Status
			if item.Status == "done" || item.Status == "completed" {
				stBadge = greenBold(item.Status)
			} else if item.Status == "in_progress" {
				stBadge = yellowBold(item.Status)
			} else if item.Status == "blocked" {
				stBadge = redBold(item.Status)
			}

			claimed := item.ClaimedBy
			if claimed == "" || claimed == "<nil>" {
				claimed = dim("unassigned")
			}

			tTitle := item.Title
			if len(tTitle) > titleW {
				tTitle = tTitle[:titleW-3] + "..."
			}

			b.WriteString(fmt.Sprintf("  %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
				bliIDW, item.ID, prioW, item.Priority, stW, stBadge, claimW, claimed, tTitle))
		}
	}
}

// renderMetricsTab renders kernel telemetry, command metrics, scheduler health, and lock hygiene.
func renderMetricsTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	b.WriteString(whiteBold("📊 Knowledge Kernel Telemetry & Resource Hygiene\n"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	// Resource Hygiene Indicators
	h := m.Hygiene
	b.WriteString(fmt.Sprintf("Resource Hygiene: %s process objects │ %s object kinds │ %s stream files │ %s active stream lanes\n",
		greenBold(fmt.Sprintf("%d", h.ProcessObjectCount)),
		whiteBold(fmt.Sprintf("%d", h.KindCount)),
		cyanBold(fmt.Sprintf("%d", h.StreamFileCount)),
		yellowBold(fmt.Sprintf("%d", h.ActiveStreams))))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	// TSDB Telemetry Section
	if m.TSDB != nil {
		b.WriteString(whiteBold("Time-Series Database (TSDB) Engine & Performance:\n"))
		b.WriteString(fmt.Sprintf("  Storage: .zqk/scheduler/tsdb (%s points across %d files, %s) │ Chunk Store: %d chunks (%s)\n",
			cyanBold(fmt.Sprintf("%d", m.TSDB.TotalPoints)), m.TSDB.TotalFiles, m.TSDB.DiskSizeStr, m.TSDB.ChunkStats.TotalChunks, m.TSDB.ChunkStats.DiskSizeStr))

		if len(m.TSDB.JobSummaries) > 0 {
			b.WriteString("  " + dim("Job Execution Latency & Reliability (from TSDB):") + "\n")
			tsdbIDW := 24
			tsdbRunsW := 5
			tsdbSuccW := 6
			tsdbFailW := 5
			tsdbAvgW := 10
			tsdbMinMaxW := 16
			tsdbLastW := 9
			b.WriteString(fmt.Sprintf("  %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
				tsdbIDW, "JOB IDENTIFIER", tsdbRunsW, "RUNS", tsdbSuccW, "SUCC", tsdbFailW, "FAIL", tsdbAvgW, "AVG LAT", tsdbMinMaxW, "MIN / MAX", tsdbLastW, "LAST RUN", "LATENCY TREND"))
			b.WriteString("  " + dim(strings.Repeat("─", w-4)) + "\n")

			limit := 7
			if len(m.TSDB.JobSummaries) < limit {
				limit = len(m.TSDB.JobSummaries)
			}
			for i := 0; i < limit; i++ {
				j := m.TSDB.JobSummaries[i]
				lastRunStr := "--:--:--"
				if !j.LastRunAt.IsZero() {
					lastRunStr = j.LastRunAt.Format("15:04:05")
				}
				failBadge := fmt.Sprintf("%d", j.Failures)
				if j.Failures > 0 {
					failBadge = redBold(failBadge)
				}
				displayID := j.JobID
				if len(displayID) > tsdbIDW {
					displayID = displayID[:tsdbIDW-3] + "..."
				}
				avgStr := formatDurationMs(j.AvgDurationMs)
				minMaxStr := fmt.Sprintf("%s / %s", formatDurationMs(j.MinDurationMs), formatDurationMs(j.MaxDurationMs))

				b.WriteString(fmt.Sprintf("  %-*s │ %-*d │ %-*d │ %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
					tsdbIDW, displayID, tsdbRunsW, j.Executions, tsdbSuccW, j.Successes, tsdbFailW, failBadge, tsdbAvgW, avgStr, tsdbMinMaxW, minMaxStr, tsdbLastW, lastRunStr, j.Sparkline))
			}
		}
		b.WriteString("\n")
	}

	// Command Telemetry
	b.WriteString(whiteBold("Command Execution Telemetry (command_metric):\n"))
	if len(m.CommandMetrics) == 0 {
		b.WriteString(dim("  [No command execution telemetry recorded yet]\n\n"))
	} else {
		cmdW := 22
		execW := 8
		durW := 12
		lastW := 19
		b.WriteString(fmt.Sprintf("  %-*s │ %-*s │ %-*s │ %-*s │ %s\n",
			cmdW, "COMMAND", execW, "CALLS", durW, "DURATION", lastW, "LAST RUN", "STATUS"))
		b.WriteString("  " + dim(strings.Repeat("─", w-4)) + "\n")
		for _, cm := range m.CommandMetrics {
			b.WriteString(fmt.Sprintf("  %-*s │ %-*d │ %-*s │ %-*s │ %s\n",
				cmdW, cm.CommandName, execW, cm.ExecCount, durW, cm.AvgDuration, lastW, cm.LastRunAt, greenBold(cm.Status)))
		}
		b.WriteString("\n")
	}

	// Scheduler Health Telemetry
	if len(m.SchedulerHealth) > 0 {
		b.WriteString(whiteBold("Scheduler Health Telemetry (scheduler_health_metric):\n"))
		for _, sh := range m.SchedulerHealth {
			st := greenBold(sh.Status)
			if sh.Failures > 0 {
				st = redBold(fmt.Sprintf("%s (%d failures)", sh.Status, sh.Failures))
			}
			b.WriteString(fmt.Sprintf("  • %-16s Status: %s │ Heartbeat: %s │ Total Executions: %d\n",
				sh.ID, st, sh.HeartbeatAt, sh.Executions))
		}
		b.WriteString("\n")
	}

	// Lock & Concurrency Contention
	if len(m.LockMetrics) > 0 {
		b.WriteString(whiteBold("Lock & Concurrency Metrics (file_lock_metric):\n"))
		for _, lm := range m.LockMetrics {
			b.WriteString(fmt.Sprintf("  • Target Kind: %-18s Contention: %-4d Avg Duration: %-10s Status: %s\n",
				lm.TargetKind, lm.Contention, lm.Duration, greenBold(lm.Status)))
		}
		b.WriteString("\n")
	}

	// Quality & Aggregation
	if len(m.QualityMetrics) > 0 {
		b.WriteString(whiteBold("Quality & Stream Aggregation Metrics:\n"))
		for _, qm := range m.QualityMetrics {
			b.WriteString(fmt.Sprintf("  • %-18s Metric: %-16s Value: %-14s Status: %s\n",
				qm.ID, qm.MetricType, cyanBold(qm.Value), greenBold(qm.Status)))
		}
	}
}

func renderSchedulerTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(whiteBold("⏱️ Autonomous Scheduler & Background Daemons\n"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	if len(m.SchedulerJobs) == 0 {
		b.WriteString(dim("  [No scheduler jobs registered or scheduler not initialized]\n\n"))
		return
	}

	idW := 28
	schW := 10
	lastW := 19
	nextW := 19
	if w < 90 {
		idW = 20
		schW = 8
	}

	b.WriteString(fmt.Sprintf("%-*s │ %-*s │ %-*s │ %-*s │ %s\n",
		idW, "JOB IDENTIFIER", schW, "SCHEDULE", lastW, "LAST RUN", nextW, "NEXT RUN", "STATUS"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	for _, job := range m.SchedulerJobs {
		stBadge := greenBold(job.Status)
		if job.Status == "failed" || job.Status == "error" {
			stBadge = redBold(job.Status)
		} else if job.Status == "paused" {
			stBadge = yellowBold(job.Status)
		}

		jID := job.ID
		if len(jID) > idW {
			jID = jID[:idW-3] + "..."
		}

		sch := job.Schedule
		if len(sch) > schW {
			sch = sch[:schW-3] + "..."
		}

		b.WriteString(fmt.Sprintf("%-*s │ %-*s │ %-*s │ %-*s │ %s\n",
			idW, jID, schW, sch, lastW, job.LastRunAt, nextW, job.NextRunAt, stBadge))
	}
}

func renderFooter(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")
	b.WriteString(dim("Legend: ") + "⚡ State │ 📜 Audit │ 🤖 Swarm │ 📋 PM/Process │ 📊 Metrics │ ⏱️ Scheduler\n")
	b.WriteString(whiteBold("[Tab / 1-6]") + " Switch View  " +
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
		{"change_journal_entry", "📦", "CHA"},
		{"agent_instruction", "📨", "AGI"},
		{"process_lifecycle", "🔄", "PRC"},
		{"agent_task", "🤖", "ATK"},
		{"backlog_item", "📋", "BLI"},
		{"requirement", "🎯", "REQ"},
		{"test_case", "🧪", "TCA"},
		{"scheduler_job", "⚙️", "SCH"},
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

func formatDurationMs(ms float64) string {
	if ms < 1.0 {
		return fmt.Sprintf("%.2fms", ms)
	}
	if ms < 1000.0 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fs", ms/1000.0)
}
