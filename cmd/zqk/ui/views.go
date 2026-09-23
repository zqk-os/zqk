package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/cmd/zqk/ui/tds"
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
		{TabScheduler, "6: ⏱️  Scheduler"},
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
			summary := mut.DiffSummary
			if summary == "" {
				summary = mut.ChangeType
			}

			b.WriteString(fmt.Sprintf("[%s] │ %s │ %s │ %s\n",
				tStr,
				tds.PadRight(badge, eventW),
				tds.PadRight(tds.TruncateVisible(ref, refW, "…"), refW),
				tds.TruncateVisible(summary, sumW, "…")))
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

			op := aud.Operation
			if op == "" {
				op = aud.ChangeType
			}

			detail := aud.DiffSummary
			if detail == "" {
				detail = "--"
			}

			b.WriteString(fmt.Sprintf("[%s] │ %s │ %s │ %s │ %s\n",
				tStr,
				tds.PadRight(tds.TruncateVisible(act, actorW, "…"), actorW),
				tds.PadRight(tds.TruncateVisible(op, opW, "…"), opW),
				tds.PadRight(tds.TruncateVisible(aud.ObjectRef, refW, "…"), refW),
				tds.TruncateVisible(detail, detailW, "…")))
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
		bliTable := tds.NewTable(w).
			AddColumn("BLI ID", tds.AlignLeft, 14, 0.16).
			AddColumn("PRIO", tds.AlignCenter, 6, 0.08).
			AddColumn("STATUS", tds.AlignCenter, 12, 0.14).
			AddColumn("CLAIMED BY", tds.AlignLeft, 16, 0.20).
			AddColumn("TITLE", tds.AlignLeft, 25, 0.42)

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

			bliTable.AddRow(
				item.ID,
				item.Priority,
				stBadge,
				claimed,
				item.Title,
			)
		}
		b.WriteString(bliTable.Render())
		b.WriteString("\n")
	}
}

// renderMetricsTab renders kernel telemetry, command metrics, scheduler health, and lock hygiene.
func renderMetricsTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	// 1. System Vitals & Resource Hygiene Card (using TDS Panel)
	var vitalsLines []string
	if m.TSDB != nil && m.TSDB.HygieneStats.MaxFileDescriptors > 0 {
		h := m.TSDB.HygieneStats
		fdItem := tds.StatItem{
			Label: "File Descriptors",
			Value: fmt.Sprintf("%d / %d (%.2f%%)", h.OpenFileDescriptors, h.MaxFileDescriptors, h.FDUsagePercent),
			Extra: tds.Badge(h.Status),
		}
		storItem := tds.StatItem{
			Label: "Storage Volume",
			Value: h.StorageSizeStr,
			Extra: dim(fmt.Sprintf("(%d files)", h.TotalStorageFiles)),
		}
		vitalsLines = append(vitalsLines, tds.StatRow([]tds.StatItem{fdItem, storItem}, w-4))

		lockItem := tds.StatItem{
			Label: "Lock Contention",
			Value: fmt.Sprintf("%d stale locks", h.StaleLocksCount),
			Extra: tds.Badge(ternary(h.StaleLocksCount == 0, "HEALTHY", "ATTENTION")),
		}
		draftItem := tds.StatItem{
			Label: "Draft Plane",
			Value: fmt.Sprintf("%d awaiting crossing", h.DraftObjectsCount),
			Extra: tds.Badge(ternary(h.DraftObjectsCount == 0, "OK", "PENDING")),
		}
		vitalsLines = append(vitalsLines, tds.StatRow([]tds.StatItem{lockItem, draftItem}, w-4))
	} else {
		h := m.Hygiene
		row1 := tds.StatItem{
			Label: "Resource Hygiene",
			Value: fmt.Sprintf("%d process objects", h.ProcessObjectCount),
			Extra: dim(fmt.Sprintf("(%d kinds)", h.KindCount)),
		}
		row2 := tds.StatItem{
			Label: "Active Streams",
			Value: fmt.Sprintf("%d lanes", h.ActiveStreams),
			Extra: dim(fmt.Sprintf("(%d files)", h.StreamFileCount)),
		}
		vitalsLines = append(vitalsLines, tds.StatRow([]tds.StatItem{row1, row2}, w-4))
	}
	b.WriteString(tds.Panel("📊 Knowledge Kernel Telemetry & Resource Hygiene", vitalsLines, w, tds.BorderRounded))
	b.WriteString("\n")

	// 2. Kernel Object Volumes (CAS Storage)
	if m.TSDB != nil && len(m.TSDB.KindVolumes) > 0 {
		b.WriteString(tds.SectionDivider("KERNEL OBJECT INVENTORY (CAS STORAGE)", w))
		var items []tds.StatItem
		for _, kv := range m.TSDB.KindVolumes {
			items = append(items, tds.StatItem{
				Label: kv.Kind,
				Value: fmt.Sprintf("%d", kv.CurrentCount),
			})
		}
		for i := 0; i < len(items); i += 4 {
			end := i + 4
			if end > len(items) {
				end = len(items)
			}
			b.WriteString("  " + tds.StatRow(items[i:end], w-4) + "\n")
		}
		b.WriteString("\n")
	}

	// 3. Stream Ingestion Velocity
	if m.TSDB != nil && len(m.TSDB.StreamRates) > 0 {
		b.WriteString(tds.SectionDivider("STREAM INGESTION VOLUME", w))
		var sItems []tds.StatItem
		for _, sr := range m.TSDB.StreamRates {
			sItems = append(sItems, tds.StatItem{
				Label: sr.StreamName,
				Value: fmt.Sprintf("%d files", sr.TotalFiles),
			})
		}
		for i := 0; i < len(sItems); i += 3 {
			end := i + 3
			if end > len(sItems) {
				end = len(sItems)
			}
			b.WriteString("  " + tds.StatRow(sItems[i:end], w-4) + "\n")
		}
		b.WriteString("\n")
	}

	// 4. CLI Command Velocity
	if m.TSDB != nil && len(m.TSDB.TopCommands) > 0 {
		b.WriteString(tds.SectionDivider("TOP CLI COMMAND VELOCITY (command_metrics)", w))
		cmdTable := tds.NewTable(w).
			AddColumn("COMMAND", tds.AlignLeft, 24, 0.45).
			AddColumn("RUNS", tds.AlignRight, 6, 0.12).
			AddColumn("AVG LATENCY", tds.AlignRight, 10, 0.18).
			AddColumn("FAILURES", tds.AlignRight, 8, 0.10).
			AddColumn("STATUS", tds.AlignCenter, 10, 0.15)

		for _, cm := range m.TSDB.TopCommands {
			errStr := fmt.Sprintf("%d", cm.Failures)
			stBadge := tds.Badge("PASS")
			if cm.Failures > 0 {
				errStr = redBold(errStr)
				stBadge = tds.Badge("WARN")
			}
			cmdTable.AddRow(
				cm.Command,
				fmt.Sprintf("%d", cm.Invocations),
				formatDurationMs(cm.AvgLatencyMs),
				errStr,
				stBadge,
			)
		}
		b.WriteString(cmdTable.Render())
		b.WriteString("\n")
	} else if len(m.CommandMetrics) > 0 {
		b.WriteString(tds.SectionDivider("COMMAND EXECUTION TELEMETRY (command_metric)", w))
		cmdTable := tds.NewTable(w).
			AddColumn("COMMAND", tds.AlignLeft, 22, 0.35).
			AddColumn("CALLS", tds.AlignRight, 8, 0.15).
			AddColumn("DURATION", tds.AlignRight, 12, 0.20).
			AddColumn("LAST RUN", tds.AlignCenter, 19, 0.30)
		for _, cm := range m.CommandMetrics {
			cmdTable.AddRow(
				cm.CommandName,
				fmt.Sprintf("%d", cm.ExecCount),
				cm.AvgDuration,
				cm.LastRunAt,
			)
		}
		b.WriteString(cmdTable.Render())
		b.WriteString("\n")
	}

	// 5. Scheduler Health Telemetry
	if len(m.SchedulerHealth) > 0 {
		b.WriteString(tds.SectionDivider("SCHEDULER HEALTH TELEMETRY (scheduler_health_metric)", w))
		for _, sh := range m.SchedulerHealth {
			st := greenBold(sh.Status)
			if sh.Failures > 0 {
				st = redBold(fmt.Sprintf("%s (%d failures)", sh.Status, sh.Failures))
			}
			b.WriteString(fmt.Sprintf("  • %-16s Status: %s │ Heartbeat: %s │ Executions: %d\n",
				sh.ID, st, sh.HeartbeatAt, sh.Executions))
		}
		b.WriteString("\n")
	}

	// 6. Scheduler Job Execution Performance & Latency Matrix
	if m.TSDB != nil && len(m.TSDB.JobSummaries) > 0 {
		b.WriteString(tds.SectionDivider("SCHEDULER JOB LATENCY & RELIABILITY MATRIX (TSDB)", w))
		schedTable := tds.NewTable(w).
			AddColumn("JOB IDENTIFIER", tds.AlignLeft, 22, 0.30).
			AddColumn("RUNS", tds.AlignRight, 5, 0.08).
			AddColumn("SUCC", tds.AlignRight, 5, 0.08).
			AddColumn("FAIL", tds.AlignRight, 5, 0.08).
			AddColumn("AVG LAT", tds.AlignRight, 9, 0.14).
			AddColumn("MIN / MAX", tds.AlignRight, 14, 0.16).
			AddColumn("LAST RUN", tds.AlignCenter, 8, 0.08).
			AddColumn("TREND", tds.AlignCenter, 8, 0.08)

		limit := 8
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
			schedTable.AddRow(
				j.JobID,
				fmt.Sprintf("%d", j.Executions),
				fmt.Sprintf("%d", j.Successes),
				failBadge,
				formatDurationMs(j.AvgDurationMs),
				fmt.Sprintf("%s / %s", formatDurationMs(j.MinDurationMs), formatDurationMs(j.MaxDurationMs)),
				lastRunStr,
				j.Sparkline,
			)
		}
		b.WriteString(schedTable.Render())
		b.WriteString("\n")
	}
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

func renderSchedulerTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(whiteBold("⏱️  Autonomous Scheduler & Background Daemons\n"))
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	if len(m.SchedulerJobs) == 0 {
		b.WriteString(dim("  [No scheduler jobs registered or scheduler not initialized]\n\n"))
		return
	}

	schedTable := tds.NewTable(w).
		AddColumn("JOB IDENTIFIER", tds.AlignLeft, 24, 0.35).
		AddColumn("SCHEDULE", tds.AlignLeft, 10, 0.15).
		AddColumn("LAST RUN", tds.AlignCenter, 19, 0.20).
		AddColumn("NEXT RUN", tds.AlignCenter, 19, 0.20).
		AddColumn("STATUS", tds.AlignCenter, 10, 0.10)

	for _, job := range m.SchedulerJobs {
		stBadge := greenBold(job.Status)
		if job.Status == "failed" || job.Status == "error" {
			stBadge = redBold(job.Status)
		} else if job.Status == "paused" {
			stBadge = yellowBold(job.Status)
		}

		schedTable.AddRow(
			job.ID,
			job.Schedule,
			job.LastRunAt,
			job.NextRunAt,
			stBadge,
		)
	}

	b.WriteString(schedTable.Render())
	b.WriteString("\n")
}

func renderFooter(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")
	b.WriteString(dim("Legend: ") + "⚡ State │ 📜 Audit │ 🤖 Swarm │ 📋 PM/Process │ 📊 Metrics │ ⏱️  Scheduler\n")
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
