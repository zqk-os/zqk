package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/zqk-os/zqk/cmd/zqk/state"
	"github.com/zqk-os/zqk/pkg/tui/tds"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

var (
	cyanBold   = color.New(color.FgCyan, color.Bold).SprintFunc()
	cyan       = color.New(color.FgCyan).SprintFunc()
	whiteBold  = color.New(color.FgWhite, color.Bold).SprintFunc()
	yellowBold = color.New(color.FgYellow, color.Bold).SprintFunc()
	yellow     = color.New(color.FgYellow).SprintFunc()
	greenBold  = color.New(color.FgGreen, color.Bold).SprintFunc()
	redBold    = color.New(color.FgRed, color.Bold).SprintFunc()
	dim        = color.New(color.Faint).SprintFunc()
	reverse    = color.New(color.ReverseVideo).SprintFunc()
)

// Render formats the entire TUI screen into an ANSI string.
func Render(m *UIModel) string {
	// If a modal drill-down inspection is active, render the modal overlay
	if m.DetailModal != nil {
		return renderDetailModal(m)
	}

	var headerBuf strings.Builder
	renderHeader(&headerBuf, m)

	var bodyBuf strings.Builder
	switch m.ActiveTab {
	case TabState:
		renderStateTab(&bodyBuf, m)
	case TabAudit:
		renderAuditTab(&bodyBuf, m)
	case TabSwarm:
		renderSwarmTab(&bodyBuf, m)
	case TabPM:
		renderPMTab(&bodyBuf, m)
	case TabMetrics:
		renderMetricsTab(&bodyBuf, m)
	case TabScheduler:
		renderSchedulerTab(&bodyBuf, m)
	case TabQA:
		renderQATab(&bodyBuf, m)
	case TabHealth:
		renderHealthTab(&bodyBuf, m)
	default:
		renderStateTab(&bodyBuf, m)
	}

	var footerBuf strings.Builder
	renderFooter(&footerBuf, m)

	headerStr := headerBuf.String()
	bodyStr := bodyBuf.String()
	footerStr := footerBuf.String()

	if m.Height > 0 {
		var headerLines []string
		if len(headerStr) > 0 {
			headerLines = strings.Split(headerStr, "\n")
			if len(headerLines) > 0 && headerLines[len(headerLines)-1] == "" {
				headerLines = headerLines[:len(headerLines)-1]
			}
		}

		var footerLines []string
		if len(footerStr) > 0 {
			footerLines = strings.Split(footerStr, "\n")
			if len(footerLines) > 0 && footerLines[len(footerLines)-1] == "" {
				footerLines = footerLines[:len(footerLines)-1]
			}
		}

		var bodyLines []string
		if len(bodyStr) > 0 {
			bodyLines = strings.Split(bodyStr, "\n")
			if len(bodyLines) > 0 && bodyLines[len(bodyLines)-1] == "" {
				bodyLines = bodyLines[:len(bodyLines)-1]
			}
		}

		keepTop := len(headerLines)
		keepBottom := len(footerLines)
		middleBudget := m.Height - keepTop - keepBottom
		if middleBudget < 1 {
			middleBudget = 1
		}

		w := m.Width
		if w < 70 {
			w = 80
		}

		var resultLines []string
		if len(bodyLines) > middleBudget {
			start := 0
			if m.ScrollOffset > 0 && (m.ActiveTab == TabState || m.ActiveTab == TabAudit) {
				start = m.ScrollOffset
				if start > len(bodyLines)-middleBudget {
					start = len(bodyLines) - middleBudget
				}
			}
			end := start + middleBudget
			if end > len(bodyLines) {
				end = len(bodyLines)
			}
			middleLines := bodyLines[start:end]
			resultLines = append(resultLines, headerLines...)
			resultLines = append(resultLines, middleLines...)
			resultLines = append(resultLines, footerLines...)
		} else {
			resultLines = append(resultLines, headerLines...)
			resultLines = append(resultLines, bodyLines...)
			resultLines = append(resultLines, footerLines...)
		}

		if len(resultLines) > m.Height {
			resultLines = resultLines[:m.Height]
		}
		for i := range resultLines {
			if tds.VisibleWidth(resultLines[i]) > w {
				resultLines[i] = tds.TruncateVisible(resultLines[i], w, "")
			}
		}
		return strings.Join(resultLines, "\n")
	}

	return headerStr + bodyStr + footerStr
}

func renderHeader(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	// 1. Jedi (Zen) Mode: collapse double-box banner, tabs, and dynamic message bar.
	// Only render interactive search prompts when search is actively engaged or filtered.
	if m.EditorProfile == ProfileJedi {
		renderSearchBar(b, m, w)
		return
	}

	// Helper for building tab buttons
	type tabEntry struct {
		idx     int
		full    string
		tight   string
		compact string
	}
	tabs := []tabEntry{
		{TabState, "1: ⚡ State", "1: ⚡ State", "1:State"},
		{TabAudit, "2: 📜 Audit", "2: 📜 Audit", "2:Audit"},
		{TabSwarm, "3: 🤖 Swarm", "3: 🤖 Swarm", "3:Swarm"},
		{TabPM, "4: 📋 PM", "4: 📋 PM", "4:PM"},
		{TabMetrics, "5: 📊 Metrics", "5: 📊 Metrics", "5:Metrics"},
		{TabScheduler, "6: ⏱️ Sched", "6: ⏱️ Sched", "6:Sched"},
		{TabQA, "7: 🧪 QA", "7: 🧪 QA", "7:QA"},
		{TabHealth, "8: 🛡️ Health", "8: 🛡️ Health", "8:Health"},
	}

	buildTabLine := func(mode string, sep string) string {
		var tabStrs []string
		for _, t := range tabs {
			var label string
			switch mode {
			case "full":
				label = t.full
			case "tight":
				label = t.tight
			default:
				label = t.compact
			}
			if t.idx == m.ActiveTab {
				tabStrs = append(tabStrs, reverse(fmt.Sprintf("[%s]", label)))
			} else {
				tabStrs = append(tabStrs, dim(fmt.Sprintf(" %s ", label)))
			}
		}
		return strings.Join(tabStrs, sep)
	}

	// 2. Pro Mode: compact 1-line top title + tab bar + profile tag
	if m.EditorProfile == ProfilePro {
		var tabLine string
		if w >= 115 {
			tabLine = buildTabLine("full", "│")
		} else if w >= 90 {
			tabLine = buildTabLine("tight", "│")
		} else {
			tabLine = buildTabLine("compact", "│")
		}

		proHeader := cyanBold("⚡ ZQK") + " " + yellowBold("[PRO]") + " │ " + tabLine
		if tds.VisibleWidth(proHeader) > w {
			proHeader = tds.TruncateVisible(proHeader, w, "")
		}
		b.WriteString(tds.PadRight(proHeader, w) + "\n")
		b.WriteString(dim(strings.Repeat("─", w)) + "\n")

		renderDynamicMessage(b, m, w)
		b.WriteString(dim(strings.Repeat("─", w)) + "\n")

		renderSearchBar(b, m, w)
		return
	}

	// 3. Newb Mode (Default): full decorative double-box banner + full tabs bar
	b.WriteString("╔" + strings.Repeat("═", w-2) + "╗\n")
	title := "ZQK KNOWLEDGE KERNEL — MISSION CONTROL CONSOLE"
	innerW := w - 2
	if tds.VisibleWidth(title) > innerW-2 {
		title = tds.TruncateVisible(title, innerW-4, "…")
	}
	titleFormatted := cyanBold(title)
	b.WriteString("║" + tds.PadCenter(titleFormatted, innerW) + "║\n")
	b.WriteString("╚" + strings.Repeat("═", w-2) + "╝\n")

	var tabLine string
	if w >= 120 {
		tabLine = buildTabLine("full", " │ ")
	} else if w >= 110 {
		tabLine = buildTabLine("full", "│")
	} else if w >= 92 {
		tabLine = buildTabLine("compact", " │ ")
	} else {
		tabLine = buildTabLine("compact", "│")
	}
	if tds.VisibleWidth(tabLine) > w {
		tabLine = tds.TruncateVisible(tabLine, w, "")
	}
	b.WriteString(tds.PadRight(tabLine, w) + "\n")
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	renderDynamicMessage(b, m, w)
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	renderSearchBar(b, m, w)
}

func renderDynamicMessage(b *strings.Builder, m *UIModel, w int) {
	msg := m.DynamicMessage
	if msg == "" {
		msg = "System operating normally — ambient telemetry stream active"
	}
	prefix := cyanBold("🔔 MESSAGE: ")
	prefixW := tds.VisibleWidth("🔔 MESSAGE: ")
	maxMsgW := w - prefixW - 2
	if maxMsgW < 10 {
		maxMsgW = 10
	}
	msgClean := tds.TruncateVisible(msg, maxMsgW, "…")

	var coloredMsg string
	if strings.Contains(msg, "✓") {
		coloredMsg = greenBold(msgClean)
	} else if strings.Contains(msg, "⚡") || strings.Contains(msg, "⏳") {
		coloredMsg = yellowBold(msgClean)
	} else if strings.Contains(msg, "✗") || strings.Contains(msg, "FAIL") || strings.Contains(msg, "ERR") {
		coloredMsg = redBold(msgClean)
	} else {
		coloredMsg = cyan(msgClean)
	}

	dynamicLine := prefix + coloredMsg
	if tds.VisibleWidth(dynamicLine) > w {
		dynamicLine = tds.TruncateVisible(dynamicLine, w, "")
	}
	b.WriteString(tds.PadRight(dynamicLine, w) + "\n")
}

func renderSearchBar(b *strings.Builder, m *UIModel, w int) {
	if m.IsSearching {
		searchPrompt := cyanBold("🔍 SEARCH: ") + "[/" + whiteBold(m.SearchBuffer) + cyanBold("█") + "]" + dim("  (Press Enter to lock search, Esc to cancel)")
		if tds.VisibleWidth(searchPrompt) > w {
			searchPrompt = tds.TruncateVisible(searchPrompt, w, "")
		}
		b.WriteString(tds.PadRight(searchPrompt, w) + "\n")
		b.WriteString(dim(strings.Repeat("─", w)) + "\n")
	} else if m.SearchQuery != "" {
		matchCount := m.GetCurrentRowCount()
		matchStr := fmt.Sprintf("%d match", matchCount)
		if matchCount != 1 {
			matchStr += "es"
		}
		filterBanner := cyanBold("🔍 SEARCH: ") + "\"" + yellowBold(m.SearchQuery) + "\" " + greenBold(fmt.Sprintf("(%s)", matchStr)) +
			dim(" │ [/] Edit │ [n/N] Next/Prev │ [g/G] Top/Bottom │ [Esc] Clear")
		if tds.VisibleWidth(filterBanner) > w {
			filterBanner = tds.TruncateVisible(filterBanner, w, "")
		}
		b.WriteString(tds.PadRight(filterBanner, w) + "\n")
		b.WriteString(dim(strings.Repeat("─", w)) + "\n")
	}
}

// renderStateTab renders real-time state mutations, instructions, and lifecycles (excluding high-volume audit logs).
func renderStateTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	mutations := m.GetVisibleMutations()
	kindCounts := make(map[string]int)
	for _, mut := range mutations {
		parts := strings.Split(mut.ObjectRef, ":")
		if len(parts) > 0 && parts[0] != "" {
			kindCounts[parts[0]]++
		}
	}

	sparkline := generateSparkline(len(mutations))
	symbolCounters := formatSymbolCounters(kindCounts)

	scrollMode := greenBold("[AUTO-SCROLL: ON]")
	if !m.AutoScroll {
		scrollMode = yellowBold(fmt.Sprintf("[PAUSED: +%d]", m.ScrollOffset))
	}

	vitals := []tds.StatItem{
		{Label: "Status", Value: greenBold("ENFORCING")},
		{Label: "Buffer", Value: fmt.Sprintf("%d state mutations", len(mutations))},
		{Label: "Rate", Value: sparkline},
		{Label: "Mode", Value: scrollMode},
	}
	b.WriteString(tds.Panel("State Engine & Real-Time Mutation Journal (WAL)", []string{
		tds.StatRow(vitals, w-4),
		tds.StatRow([]tds.StatItem{
			{Label: "Types", Value: symbolCounters},
		}, w-4),
	}, w, tds.BorderRounded))
	b.WriteString("\n")

	stateTable := tds.NewTable(w).
		AddColumn("TIME", tds.AlignLeft, 12, 0.12).
		AddColumn("EVENT", tds.AlignLeft, 14, 0.16).
		AddColumn("OBJECT REF", tds.AlignLeft, 28, 0.32).
		AddColumn("SUMMARY / DIFF", tds.AlignLeft, 30, 0.40)

	availRows := m.Height - 12 + m.ProfileSpacingBonus()
	if availRows < 6 {
		availRows = 6
	}

	total := len(mutations)
	var visible []state.JournalMutation

	if total == 0 {
		msg := "[No state mutations recorded in stream buffer yet]"
		if m.SearchQuery != "" {
			msg = fmt.Sprintf("[No state mutations matching search query \"%s\"]", m.SearchQuery)
		}
		stateTable.AddRow("-", "-", "-", msg)
	} else {
		start := 0
		if m.AutoScroll {
			start = total - availRows
			if start < 0 {
				start = 0
			}
			visible = mutations[start:]
		} else {
			end := total - m.ScrollOffset
			if end > total {
				end = total
			}
			start = end - availRows
			if start < 0 {
				start = 0
			}
			visible = mutations[start:end]
		}

		for i, mut := range visible {
			tStr := "--:--:--"
			if mut.CreatedAt > 0 {
				tStr = time.Unix(mut.CreatedAt, 0).Format("15:04:05")
			}

			badge := state.FormatEventBadge(mut.ChangeType)
			ref := strings.ReplaceAll(strings.ReplaceAll(mut.ObjectRef, "\r", ""), "\n", " ")
			summary := strings.ReplaceAll(strings.ReplaceAll(mut.DiffSummary, "\r", ""), "\n", " ")
			if summary == "" {
				summary = mut.ChangeType
			}

			actualIdx := start + i
			isSel := actualIdx == m.SelectedIndex || (m.AutoScroll && (m.SelectedIndex < start || m.SelectedIndex >= total) && actualIdx == total-1)
			timeCell := tds.RowCursor(isSel, fmt.Sprintf("[%s]", tStr))

			stateTable.AddRow(
				timeCell,
				badge,
				ref,
				summary,
			)
		}
	}

	b.WriteString(stateTable.Render())
}

// renderAuditTab renders the dedicated high-volume operational audit stream.
func renderAuditTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	auditEvents := m.GetVisibleAuditEvents()
	actors := make(map[string]int)
	for _, a := range auditEvents {
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

	sparkline := generateSparkline(len(auditEvents))
	scrollMode := greenBold("[AUTO-SCROLL: ON]")
	if !m.AutoScroll {
		scrollMode = yellowBold(fmt.Sprintf("[PAUSED: +%d]", m.ScrollOffset))
	}

	vitals := []tds.StatItem{
		{Label: "Stream", Value: cyanBold("audit_event")},
		{Label: "Buffer", Value: fmt.Sprintf("%d audit events", len(auditEvents))},
		{Label: "Rate", Value: sparkline},
		{Label: "Mode", Value: scrollMode},
	}
	b.WriteString(tds.Panel("Operational Audit Stream & Cryptographic Provenance", []string{
		tds.StatRow(vitals, w-4),
		tds.StatRow([]tds.StatItem{
			{Label: "Actors", Value: actorLine},
		}, w-4),
	}, w, tds.BorderRounded))
	b.WriteString("\n")

	auditTable := tds.NewTable(w).
		AddColumn("TIME", tds.AlignLeft, 10, 0.10).
		AddColumn("ACTOR", tds.AlignLeft, 14, 0.15).
		AddColumn("OPERATION", tds.AlignLeft, 14, 0.15).
		AddColumn("OBJECT REF", tds.AlignLeft, 26, 0.30).
		AddColumn("DETAILS / PAYLOAD", tds.AlignLeft, 24, 0.30)

	availRows := m.Height - 12 + m.ProfileSpacingBonus()
	if availRows < 6 {
		availRows = 6
	}

	total := len(auditEvents)
	var visible []state.JournalMutation

	if total == 0 {
		msg := "[No audit events recorded in audit stream yet]"
		if m.SearchQuery != "" {
			msg = fmt.Sprintf("[No audit events matching search query \"%s\"]", m.SearchQuery)
		}
		auditTable.AddRow("-", "-", "-", "-", msg)
	} else {
		start := 0
		if m.AutoScroll {
			start = total - availRows
			if start < 0 {
				start = 0
			}
			visible = auditEvents[start:]
		} else {
			end := total - m.ScrollOffset
			if end > total {
				end = total
			}
			start = end - availRows
			if start < 0 {
				start = 0
			}
			visible = auditEvents[start:end]
		}

		for i, aud := range visible {
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

			detail := strings.ReplaceAll(strings.ReplaceAll(aud.DiffSummary, "\r", ""), "\n", " ")
			if detail == "" {
				detail = "--"
			}
			ref := strings.ReplaceAll(strings.ReplaceAll(aud.ObjectRef, "\r", ""), "\n", " ")

			actualIdx := start + i
			isSel := actualIdx == m.SelectedIndex || (m.AutoScroll && (m.SelectedIndex < start || m.SelectedIndex >= total) && actualIdx == total-1)
			timeCell := tds.RowCursor(isSel, fmt.Sprintf("[%s]", tStr))

			auditTable.AddRow(
				timeCell,
				act,
				op,
				ref,
				detail,
			)
		}
	}

	b.WriteString(auditTable.Render())
}

func renderSwarmTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	if m.SwarmData == nil {
		b.WriteString(tds.Panel("Multi-Agent Swarm Orchestration & Throughput (MMORCH)", []string{
			"  [Swarm status initializing or storage unavailable...]",
		}, w, tds.BorderRounded))
		b.WriteString("\n")
		renderSwarmInbox(b, m, w)
		return
	}

	hint := fmt.Sprintf("%v", m.SwarmData["throughput_hint"])
	plans := fmt.Sprintf("%v", m.SwarmData["active_priority_plans"])
	tasks := fmt.Sprintf("%v", m.SwarmData["executing_agent_tasks"])
	instTotal := fmt.Sprintf("%v", m.SwarmData["agent_instructions_total"])

	// 1. Throughput Vitals Panel
	vitals := []tds.StatItem{
		{Label: "Throughput Status", Value: hint, Extra: tds.Badge(ternary(hint == "executing" || hint == "active", "ACTIVE", "PENDING"))},
		{Label: "Active Priority Plans", Value: plans},
		{Label: "Executing Agent Tasks", Value: tasks},
		{Label: "Total Instructions", Value: instTotal},
	}
	vitalsLines := []string{
		tds.StatRow(vitals[:2], w-4),
		tds.StatRow(vitals[2:], w-4),
	}
	b.WriteString(tds.Panel("Multi-Agent Swarm Orchestration & Throughput (MMORCH)", vitalsLines, w, tds.BorderRounded))
	b.WriteString("\n")

	// 2. Seating & Personas Card
	var seatingLines []string
	if personaInfo, ok := m.SwarmData["persona_skill_bound"].(map[string]any); ok {
		bound := fmt.Sprintf("%v", personaInfo["bound"])
		total := fmt.Sprintf("%v", personaInfo["total"])
		unbound := fmt.Sprintf("%v", personaInfo["unbound"])
		seatingItems := []tds.StatItem{
			{Label: "Bound Personas", Value: bound, Extra: tds.Badge("OK")},
			{Label: "Unbound Personas", Value: unbound, Extra: tds.Badge(ternary(unbound == "0", "OK", "WARN"))},
			{Label: "Total Swarm Personas", Value: total},
		}
		seatingLines = append(seatingLines, tds.StatRow(seatingItems, w-4))
	}

	if capJob, ok := m.SwarmData["cap_orchestrator_job"].(map[string]any); ok {
		id := fmt.Sprintf("%v", capJob["id"])
		present := fmt.Sprintf("%v", capJob["present"])
		capItem := []tds.StatItem{
			{Label: "CAP Orchestrator Daemon", Value: id, Extra: tds.Badge(ternary(present == "true", "ACTIVE", "WARN"))},
		}
		seatingLines = append(seatingLines, tds.StatRow(capItem, w-4))
	}
	if len(seatingLines) > 0 {
		b.WriteString(tds.Panel("Agent Swarm Seating & Continuous Autonomous Loop (CAP)", seatingLines, w, tds.BorderRounded))
		b.WriteString("\n")
	}

	// 3. Instruction Breakdown
	if instByStatus, ok := m.SwarmData["agent_instructions_by_status"].(map[string]any); ok && len(instByStatus) > 0 {
		var instItems []tds.StatItem
		for status, count := range instByStatus {
			instItems = append(instItems, tds.StatItem{
				Label: strings.ToUpper(status),
				Value: fmt.Sprintf("%v", count),
			})
		}
		var breakdownLines []string
		for i := 0; i < len(instItems); i += 4 {
			end := i + 4
			if end > len(instItems) {
				end = len(instItems)
			}
			breakdownLines = append(breakdownLines, tds.StatRow(instItems[i:end], w-4))
		}
		b.WriteString(tds.Panel("Instruction Queue & Backlog Breakdown", breakdownLines, w, tds.BorderRounded))
		b.WriteString("\n")
	}

	// 4. Operator & Swarm Inbox Section
	renderSwarmInbox(b, m, w)
}

// renderSwarmInbox renders the Swarm and Operator inbox table for correspondence and staged interrupt envelopes.
func renderSwarmInbox(b *strings.Builder, m *UIModel, w int) {
	inboxItems := m.GetVisibleInboxItems()
	b.WriteString(tds.SectionDivider("SWARM & OPERATOR INBOX (Press Enter to inspect, [a] to ack, [r] to reply)", w))
	if len(inboxItems) == 0 {
		b.WriteString("  " + dim("[No unacknowledged correspondence or staged envelopes in inbox]") + "\n\n")
		return
	}

	inboxTable := tds.NewTable(w).
		AddColumn("ITEM ID", tds.AlignLeft, 24, 0.25).
		AddColumn("TYPE", tds.AlignCenter, 14, 0.15).
		AddColumn("SENDER", tds.AlignLeft, 14, 0.15).
		AddColumn("TARGET", tds.AlignLeft, 14, 0.15).
		AddColumn("STATUS", tds.AlignCenter, 10, 0.10).
		AddColumn("SUMMARY", tds.AlignLeft, 20, 0.20)
	for i, item := range inboxItems {
		isSelected := (i == m.SelectedIndex)
		statusBadge := yellowBold(item.Status)
		if item.Status == "committed" || item.Status == "acked" {
			statusBadge = greenBold(item.Status)
		}
		summary := strings.ReplaceAll(strings.ReplaceAll(item.Summary, "\r", ""), "\n", " ")
		if len(summary) > 40 {
			summary = summary[:37] + "..."
		}
		inboxTable.AddRow(
			tds.RowCursor(isSelected, item.ID),
			item.Type,
			item.Sender,
			item.Target,
			statusBadge,
			summary,
		)
	}
	b.WriteString(inboxTable.Render())
	b.WriteString("\n")
}

// renderPMTab renders the dedicated PM & Process Admin dashboard (Strategic cascade, delivery pipeline, blockers).
func renderPMTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	mis := m.MissionTitle
	if mis == "" {
		mis = "Continuous Autonomous Development"
	}
	bs := m.BacklogSummary
	b.WriteString(fmt.Sprintf("%s │ %s: %s\n",
		whiteBold("📋 PM & Process Administration"),
		cyanBold("Mission"), cyanBold(mis)))
	b.WriteString(fmt.Sprintf("%s │ %s: %d  %s: %d  %s: %d  %s: %d  (%s claimed)\n",
		whiteBold(fmt.Sprintf("BLI Pipeline (%d)", bs.Total)),
		cyanBold("Planned"), bs.Planned,
		yellowBold("InProg"), bs.InProgress,
		redBold("Blocked"), bs.Blocked,
		greenBold("Done"), bs.Done,
		greenBold(fmt.Sprintf("%d", bs.Claimed))))

	effectiveHeight := m.Height + m.ProfileSpacingBonus()

	// 2. Priority Plans Table
	plans := m.GetVisiblePriorityPlans()
	blks := m.GetVisibleBlockers()
	backlog := m.GetVisibleBacklog()
	debt := m.GetVisibleTechnicalDebt()

	if len(plans) > 0 {
		b.WriteString(tds.SectionDivider("ACTIVE PRIORITY PLANS (Press Enter to inspect)", w))
		planTable := tds.NewTable(w).
			AddColumn("PLAN ID", tds.AlignLeft, 16, 0.22).
			AddColumn("STATUS", tds.AlignCenter, 12, 0.14).
			AddColumn("BLIS", tds.AlignRight, 6, 0.08).
			AddColumn("WORKSTREAMS", tds.AlignLeft, 14, 0.18).
			AddColumn("TITLE", tds.AlignLeft, 24, 0.38)

		limit := 2
		if effectiveHeight >= 40 {
			limit = 3
		}
		if len(plans) < limit {
			limit = len(plans)
		}
		start := 0
		if m.SelectedIndex < len(plans) && m.SelectedIndex >= limit {
			start = m.SelectedIndex - limit + 1
		}
		if start+limit > len(plans) {
			start = len(plans) - limit
		}
		if start < 0 {
			start = 0
		}
		for i := 0; i < limit && (start+i) < len(plans); i++ {
			actualIdx := start + i
			p := plans[actualIdx]
			wsStr := strings.Join(p.Workstreams, ", ")
			if wsStr == "" {
				wsStr = "core"
			}
			stBadge := yellowBold(p.Status)
			if p.Status == "complete" || p.Status == "done" {
				stBadge = greenBold(p.Status)
			}
			idCell := tds.RowCursor(actualIdx == m.SelectedIndex, p.ID)
			planTable.AddRow(
				idCell,
				stBadge,
				fmt.Sprintf("%d", p.BLICount),
				wsStr,
				p.Title,
			)
		}
		b.WriteString(planTable.Render())
	}

	// 3. Active Blockers & Risks
	if len(blks) > 0 {
		b.WriteString(tds.SectionDivider("ACTIVE RISKS & BLOCKERS (Press Enter to inspect)", w))
		blkTable := tds.NewTable(w).
			AddColumn("RISK ID", tds.AlignLeft, 16, 0.22).
			AddColumn("SEVERITY", tds.AlignCenter, 10, 0.12).
			AddColumn("STATUS", tds.AlignCenter, 10, 0.12).
			AddColumn("TITLE", tds.AlignLeft, 30, 0.54)

		limit := 2
		if effectiveHeight >= 40 {
			limit = 3
		}
		if len(blks) < limit {
			limit = len(blks)
		}
		blkOffset := len(plans)
		start := 0
		if m.SelectedIndex >= blkOffset && m.SelectedIndex < blkOffset+len(blks) {
			curr := m.SelectedIndex - blkOffset
			if curr >= limit {
				start = curr - limit + 1
			}
		}
		if start+limit > len(blks) {
			start = len(blks) - limit
		}
		if start < 0 {
			start = 0
		}
		for i := 0; i < limit && (start+i) < len(blks); i++ {
			actualBlkIdx := start + i
			blk := blks[actualBlkIdx]
			sev := redBold(blk.Severity)
			if blk.Severity == "low" || blk.Severity == "info" {
				sev = dim(blk.Severity)
			} else if blk.Severity == "medium" {
				sev = yellowBold(blk.Severity)
			}
			actualIdx := blkOffset + actualBlkIdx
			idCell := tds.RowCursor(actualIdx == m.SelectedIndex, blk.ID)
			blkTable.AddRow(
				idCell,
				sev,
				blk.Status,
				blk.Title,
			)
		}
		b.WriteString(blkTable.Render())
	}

	// 4. Recent Work Units (Backlog Items) Table
	if len(backlog) > 0 {
		b.WriteString(tds.SectionDivider("RECENT WORK UNITS (BACKLOG ITEMS)", w))
		bliTable := tds.NewTable(w).
			AddColumn("BLI ID", tds.AlignLeft, 16, 0.18).
			AddColumn("PRIO", tds.AlignCenter, 6, 0.08).
			AddColumn("STATUS", tds.AlignCenter, 12, 0.14).
			AddColumn("CLAIMANT", tds.AlignLeft, 10, 0.10).
			AddColumn("TITLE", tds.AlignLeft, 30, 0.50)

		bliLimit := 3
		if effectiveHeight >= 40 {
			bliLimit = 6
		} else if effectiveHeight >= 32 {
			bliLimit = 4
		}
		if len(backlog) < bliLimit {
			bliLimit = len(backlog)
		}

		bliOffset := len(plans) + len(blks)
		bliStart := 0
		if m.SelectedIndex >= bliOffset && m.SelectedIndex < bliOffset+len(backlog) {
			curr := m.SelectedIndex - bliOffset
			if curr >= bliLimit {
				bliStart = curr - bliLimit + 1
			}
		}
		if bliStart+bliLimit > len(backlog) {
			bliStart = len(backlog) - bliLimit
		}
		if bliStart < 0 {
			bliStart = 0
		}

		for i := 0; i < bliLimit && (bliStart+i) < len(backlog); i++ {
			actualBliIdx := bliStart + i
			item := backlog[actualBliIdx]
			stBadge := item.Status
			if item.Status == "done" || item.Status == "completed" || item.Status == "complete" {
				stBadge = greenBold(item.Status)
			} else if item.Status == "in_progress" || item.Status == "active" || item.Status == "executing" {
				stBadge = yellowBold(item.Status)
			} else if item.Status == "blocked" {
				stBadge = redBold(item.Status)
			} else if item.Status == "originated" || item.Status == "planned" || item.Status == "ready" {
				stBadge = cyanBold(item.Status)
			}

			claimed := item.ClaimedBy
			if claimed == "" || claimed == "<nil>" || claimed == "unassigned" {
				claimed = dim("-")
			}

			actualIdx := bliOffset + actualBliIdx
			isSelected := (actualIdx == m.SelectedIndex)
			idCell := tds.RowCursor(isSelected, item.ID)

			bliTable.AddRow(
				idCell,
				item.Priority,
				stBadge,
				claimed,
				item.Title,
			)
		}
		b.WriteString(bliTable.Render())
	}

	// 5. Technical Debt Items Table
	if len(debt) > 0 {
		b.WriteString(tds.SectionDivider("TECHNICAL DEBT & HYGIENE ITEMS", w))
		debtTable := tds.NewTable(w).
			AddColumn("DEBT ID", tds.AlignLeft, 16, 0.22).
			AddColumn("PRIORITY", tds.AlignCenter, 10, 0.12).
			AddColumn("CATEGORY", tds.AlignCenter, 14, 0.16).
			AddColumn("TITLE", tds.AlignLeft, 26, 0.50)

		debtLimit := 2
		if effectiveHeight >= 40 {
			debtLimit = 4
		} else if effectiveHeight >= 32 {
			debtLimit = 3
		}
		if len(debt) < debtLimit {
			debtLimit = len(debt)
		}

		debtOffset := len(plans) + len(blks) + len(backlog)
		debtStart := 0
		if m.SelectedIndex >= debtOffset {
			currDebtIdx := m.SelectedIndex - debtOffset
			if currDebtIdx >= debtLimit {
				debtStart = currDebtIdx - debtLimit + 1
			}
		}
		if debtStart+debtLimit > len(debt) {
			debtStart = len(debt) - debtLimit
		}
		if debtStart < 0 {
			debtStart = 0
		}

		for i := 0; i < debtLimit && (debtStart+i) < len(debt); i++ {
			actualDebtIdx := debtStart + i
			d := debt[actualDebtIdx]
			isSelected := (debtOffset+actualDebtIdx == m.SelectedIndex)
			idCell := tds.RowCursor(isSelected, d.ID)

			debtTable.AddRow(
				idCell,
				yellowBold(d.Priority),
				d.Category,
				d.Title,
			)
		}
		b.WriteString(debtTable.Render())
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
	b.WriteString(tds.Panel("Knowledge Kernel Telemetry & Resource Hygiene", vitalsLines, w, tds.BorderRounded))
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

		for i, cm := range m.TSDB.TopCommands {
			errStr := fmt.Sprintf("%d", cm.Failures)
			stBadge := tds.Badge("PASS")
			if cm.Failures > 0 {
				errStr = redBold(errStr)
				stBadge = tds.Badge("WARN")
			}
			isSelected := (i == m.SelectedIndex)
			cmdTable.AddRow(
				tds.RowCursor(isSelected, cm.Command),
				fmt.Sprintf("%d", cm.Invocations),
				formatDurationMs(cm.AvgLatencyMs),
				errStr,
				stBadge,
			)
		}
		b.WriteString(cmdTable.Render())
		b.WriteString("\n")
	} else if cmdMetrics := m.GetVisibleCommandMetrics(); len(cmdMetrics) > 0 {
		b.WriteString(tds.SectionDivider("COMMAND EXECUTION TELEMETRY (command_metric)", w))
		cmdTable := tds.NewTable(w).
			AddColumn("COMMAND", tds.AlignLeft, 24, 0.35).
			AddColumn("CALLS", tds.AlignRight, 8, 0.15).
			AddColumn("DURATION", tds.AlignRight, 12, 0.20).
			AddColumn("LAST RUN", tds.AlignCenter, 19, 0.30)
		for i, cm := range cmdMetrics {
			isSelected := (i == m.SelectedIndex)
			cmdTable.AddRow(
				tds.RowCursor(isSelected, cm.CommandName),
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
		shTable := tds.NewTable(w).
			AddColumn("HEALTH RECORD", tds.AlignLeft, 22, 0.30).
			AddColumn("HEARTBEAT", tds.AlignCenter, 19, 0.25).
			AddColumn("EXECUTIONS", tds.AlignRight, 10, 0.15).
			AddColumn("FAILURES", tds.AlignRight, 8, 0.15).
			AddColumn("STATUS", tds.AlignCenter, 10, 0.15)

		healthRows := m.SchedulerHealth
		// If all heartbeats are empty/zero, limit to at most 3 rows to save vertical space
		allInactive := true
		for _, sh := range healthRows {
			if sh.HeartbeatAt != "--" && sh.HeartbeatAt != "" && sh.Executions > 0 {
				allInactive = false
				break
			}
		}
		if allInactive && len(healthRows) > 3 {
			healthRows = healthRows[:3]
		}

		for _, sh := range healthRows {
			failStr := fmt.Sprintf("%d", sh.Failures)
			stBadge := tds.Badge("HEALTHY")
			if sh.Failures > 0 {
				failStr = redBold(failStr)
				stBadge = tds.Badge("DEGRADED")
			}
			shTable.AddRow(
				sh.ID,
				sh.HeartbeatAt,
				fmt.Sprintf("%d", sh.Executions),
				failStr,
				stBadge,
			)
		}
		b.WriteString(shTable.Render())
		b.WriteString("\n")
	}

	// 6. Scheduler Job Execution Performance & Latency Matrix
	if m.TSDB != nil && len(m.TSDB.JobSummaries) > 0 {
		b.WriteString(tds.SectionDivider("SCHEDULER JOB LATENCY & RELIABILITY (TSDB — % vs historical avg)", w))
		schedTable := tds.NewTable(w).
			AddColumn("JOB IDENTIFIER", tds.AlignLeft, 22, 0.28).
			AddColumn("RUNS", tds.AlignRight, 5, 0.08).
			AddColumn("SUCC", tds.AlignRight, 5, 0.08).
			AddColumn("FAIL", tds.AlignRight, 5, 0.08).
			AddColumn("AVG LAT", tds.AlignRight, 9, 0.14).
			AddColumn("MIN / MAX", tds.AlignRight, 14, 0.16).
			AddColumn("LAST RUN", tds.AlignCenter, 8, 0.08).
			AddColumn("LAT TREND", tds.AlignCenter, 10, 0.10)

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
			trendStr := j.Sparkline
			if strings.HasPrefix(trendStr, "▲") {
				trendStr = yellowBold(trendStr)
			} else if strings.HasPrefix(trendStr, "▼") {
				trendStr = greenBold(trendStr)
			} else if strings.Contains(trendStr, "STABLE") {
				trendStr = cyan(trendStr)
			}
			schedTable.AddRow(
				j.JobID,
				fmt.Sprintf("%d", j.Executions),
				fmt.Sprintf("%d", j.Successes),
				failBadge,
				formatDurationMs(j.AvgDurationMs),
				fmt.Sprintf("%s / %s", formatDurationMs(j.MinDurationMs), formatDurationMs(j.MaxDurationMs)),
				lastRunStr,
				trendStr,
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
	b.WriteString(tds.SectionDivider("Autonomous Scheduler & Background Daemons (Press [Enter] to inspect, [t] to trigger)", w))

	jobs := m.GetVisibleSchedulerJobs()
	if len(jobs) == 0 {
		if m.SearchQuery != "" {
			b.WriteString(dim(fmt.Sprintf("  [No scheduler jobs matching search query \"%s\"]\n\n", m.SearchQuery)))
		} else {
			b.WriteString(dim("  [No scheduler jobs registered or scheduler not initialized]\n\n"))
		}
		return
	}

	schedTable := tds.NewTable(w).
		AddColumn("JOB IDENTIFIER", tds.AlignLeft, 22, 0.30).
		AddColumn("SCHEDULE", tds.AlignLeft, 14, 0.20).
		AddColumn("LAST RUN", tds.AlignCenter, 19, 0.20).
		AddColumn("NEXT RUN", tds.AlignCenter, 19, 0.20).
		AddColumn("STATUS", tds.AlignCenter, 10, 0.10)

	availRows := m.Height - 12 + m.ProfileSpacingBonus()
	if availRows < 6 {
		availRows = 6
	}

	total := len(jobs)
	start := 0
	if m.SelectedIndex >= availRows {
		start = m.SelectedIndex - availRows + 1
	}
	if start+availRows > total {
		start = total - availRows
	}
	if start < 0 {
		start = 0
	}
	end := start + availRows
	if end > total {
		end = total
	}
	visible := jobs[start:end]

	for i, job := range visible {
		stBadge := greenBold(job.Status)
		if job.Status == "failed" || job.Status == "error" {
			stBadge = redBold(job.Status)
		} else if job.Status == "paused" {
			stBadge = yellowBold(job.Status)
		}

		actualIdx := start + i
		isSelected := (actualIdx == m.SelectedIndex)

		schedTable.AddRow(
			tds.RowCursor(isSelected, job.ID),
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
	// 1. Jedi (Zen) Mode: collapse footer completely to maximize terminal data space
	if m.EditorProfile == ProfileJedi {
		return
	}

	w := m.Width
	if w < 70 {
		w = 80
	}
	b.WriteString(dim(strings.Repeat("─", w)) + "\n")

	triggerActionHint := "[Keys] Action"
	if m.ActiveTab == TabScheduler {
		triggerActionHint = "[t/d] Trigger Job"
	} else if m.ActiveTab == TabQA {
		triggerActionHint = "[t] Re-scan Matrix"
	} else if m.ActiveTab == TabSwarm {
		triggerActionHint = "[a] Ack [r] Reply"
	}

	// 2. Pro Mode: compact single-line help bar
	if m.EditorProfile == ProfilePro {
		var proNavLine string
		if w >= 115 {
			proNavLine = whiteBold("[1-8/Tab]") + " Nav  " +
				whiteBold("[j/k]") + " Select  " +
				whiteBold("[g/G]") + " Top/Bot  " +
				whiteBold("[/]") + " Search  " +
				whiteBold("[Enter]") + " Inspect  " +
				whiteBold(triggerActionHint) + "  " +
				whiteBold("[z/?]") + " Profile (pro)  " +
				whiteBold("[q]") + " Exit"
		} else if w >= 85 {
			proNavLine = whiteBold("[1-8]") + " Nav  " +
				whiteBold("[j/k]") + " Select  " +
				whiteBold("[/]") + " Search  " +
				whiteBold("[Enter]") + " Inspect  " +
				whiteBold("[z/?]") + " Profile (pro)  " +
				whiteBold("[q]") + " Exit"
		} else {
			proNavLine = whiteBold("[1-8]") + " Nav  " +
				whiteBold("[j/k]") + " Select  " +
				whiteBold("[Enter]") + " Inspect  " +
				whiteBold("[q]") + " Exit"
		}
		if tds.VisibleWidth(proNavLine) > w {
			proNavLine = tds.TruncateVisible(proNavLine, w, "")
		}
		b.WriteString(tds.PadRight(proNavLine, w) + "\n")
		return
	}

	// 3. Newb Mode: full multi-line legend + detailed navigation shortcuts
	var legendLine string
	if w >= 115 {
		legendLine = dim("Legend: ") + "⚡ State │ 📜 Audit │ 🤖 Swarm │ 📋 PM │ 📊 Metrics │ ⏱️ Sched │ 🧪 QA │ 🛡️ Health/Actions"
	} else if w >= 85 {
		legendLine = dim("Tabs: ") + "1:State │ 2:Audit │ 3:Swarm │ 4:PM │ 5:Metrics │ 6:Sched │ 7:QA │ 8:Health"
	} else {
		legendLine = dim("Tabs: ") + "1:State 2:Audit 3:Swarm 4:PM 5:Metrics 6:Sched 7:QA 8:Health"
	}
	if tds.VisibleWidth(legendLine) > w {
		legendLine = tds.TruncateVisible(legendLine, w, "")
	}
	b.WriteString(tds.PadRight(legendLine, w) + "\n")

	var navLine string
	if w >= 135 {
		navLine = whiteBold("[Tab/1-8]") + " View  " +
			whiteBold("[↑/↓/j/k]") + " Select  " +
			whiteBold("[/]") + " Search  " +
			whiteBold("[Enter]") + " Inspect  " +
			whiteBold(triggerActionHint) + "  " +
			whiteBold("[Space]") + " Pause  " +
			whiteBold("[r]") + " Refresh  " +
			whiteBold("[z/?]") + " Profile (newb)  " +
			whiteBold("[q]") + " Exit"
	} else if w >= 115 {
		navLine = whiteBold("[Tab/1-8]") + " Switch View  " +
			whiteBold("[↑/↓]") + " Select  " +
			whiteBold("[Enter]") + " Inspect  " +
			whiteBold(triggerActionHint) + "  " +
			whiteBold("[z/?]") + " Profile (newb)  " +
			whiteBold("[r]") + " Refresh  " +
			whiteBold("[q]") + " Exit"
	} else if w >= 75 {
		navLine = whiteBold("[Tab/1-8]") + " View  " +
			whiteBold("[↑/↓]") + " Select  " +
			whiteBold("[Enter]") + " Inspect  " +
			whiteBold("[r]") + " Refresh  " +
			whiteBold("[q]") + " Exit"
	} else {
		navLine = whiteBold("[Tab]") + " Nav  " +
			whiteBold("[↑/↓]") + " Select  " +
			whiteBold("[Enter]") + " Inspect  " +
			whiteBold("[q]") + " Exit"
	}
	if tds.VisibleWidth(navLine) > w {
		navLine = tds.TruncateVisible(navLine, w, "")
	}
	b.WriteString(tds.PadRight(navLine, w) + "\n")
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
			parts = append(parts, fmt.Sprintf("%s%s: %d", tds.IconPad(m.symbol), m.tag, c))
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

// renderQATab renders the unified QA, test cases, and lineage traceability dashboard.
func renderQATab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	qs := m.QASummary

	// 1. Traceability & DoD Vitals Card (TDS Panel)
	dodStatus := tds.Badge("PASS")
	if qs.TotalTestCases == 0 {
		dodStatus = tds.Badge("PENDING")
	} else if !qs.DoDCompliant {
		dodStatus = tds.Badge("FAIL")
	}

	chainStatus := tds.Badge("OK")
	if qs.TotalTestCases == 0 {
		chainStatus = tds.Badge("EMPTY")
	} else if qs.IntactChains < qs.TotalTestCases {
		chainStatus = tds.Badge("WARN")
	}

	unboundStatus := tds.Badge("OK")
	if qs.UnboundCriteria > 0 {
		unboundStatus = tds.Badge("WARN")
	}

	critPct := 0.0
	if qs.TotalCriteria > 0 {
		critPct = float64(qs.SatisfiedCriteria) / float64(qs.TotalCriteria)
	}

	bliPct := 0.0
	if qs.TotalBLICount > 0 {
		bliPct = float64(qs.VerifiedBLICount) / float64(qs.TotalBLICount)
	}

	dodText := ternary(qs.DoDCompliant, "100% Intact", "Action Needed")
	if qs.TotalTestCases == 0 {
		dodText = "0% (No Test Suites)"
	}

	row1 := []tds.StatItem{
		{Label: "Traceability DoD", Value: dodText, Extra: dodStatus},
		{Label: "Intact Chains", Value: fmt.Sprintf("%d / %d", qs.IntactChains, qs.TotalTestCases), Extra: chainStatus},
		{Label: "Unbound Criteria", Value: fmt.Sprintf("%d", qs.UnboundCriteria), Extra: unboundStatus},
	}
	critText := fmt.Sprintf("%d / %d", qs.SatisfiedCriteria, qs.TotalCriteria)
	if qs.TotalCriteria > 0 {
		critText = fmt.Sprintf("%d / %d (%.0f%%)", qs.SatisfiedCriteria, qs.TotalCriteria, critPct*100)
	}
	row2 := []tds.StatItem{
		{Label: "In-Flight / Regress", Value: fmt.Sprintf("%d / %d", qs.InFlightCount, qs.RegressionCount)},
		{Label: "Criteria Met", Value: critText},
		{Label: "BLI Coverage", Value: fmt.Sprintf("%d / %d", qs.VerifiedBLICount, qs.TotalBLICount), Extra: tds.ProgressBar(bliPct, 8)},
	}

	panelLines := []string{
		tds.StatRow(row1, w-4),
		tds.StatRow(row2, w-4),
	}
	b.WriteString(tds.Panel("QA, Verification & Lineage Traceability (DoD Gate)", panelLines, w, tds.BorderRounded))
	b.WriteString("\n")

	// 2. Active Test Cases Table
	testCases := m.GetVisibleTestCases()
	if len(testCases) == 0 {
		if m.SearchQuery != "" {
			b.WriteString(yellow(fmt.Sprintf("  ⚠  No test cases matching search query \"%s\".\n\n", m.SearchQuery)))
		} else {
			b.WriteString(yellow("  ⚠  No active test case objects discovered in test_dashboard_lite.json or CAS storage.\n"))
			b.WriteString(dim(fmt.Sprintf("     To generate test suites and link criteria, run '%s' or '%s'.\n\n", paths.CLIInvocation("test bind"), paths.CLIInvocation("workflow whats-next"))))
		}
		return
	}

	b.WriteString(tds.SectionDivider("TEST SUITES & DOWNWARD TRACEABILITY (Press Enter to inspect, [t] to re-scan)", w))
	tcTable := tds.NewTable(w).
		AddColumn("TEST CASE ID", tds.AlignLeft, 32, 0.32).
		AddColumn("STATUS", tds.AlignCenter, 10, 0.10).
		AddColumn("LINEAGE", tds.AlignCenter, 10, 0.10).
		AddColumn("CRITERIA", tds.AlignCenter, 10, 0.10).
		AddColumn("TITLE", tds.AlignLeft, 24, 0.38)

	overhead := 18
	if len(m.UnboundCriteria) > 0 {
		overhead += 4
	}
	if len(m.RecentQAEvents) > 0 {
		overhead += 4
	}
	availRows := m.Height - overhead + m.ProfileSpacingBonus()
	if availRows < 3 {
		availRows = 3
	}

	total := len(testCases)
	start := 0
	if m.SelectedIndex >= availRows {
		start = m.SelectedIndex - availRows + 1
	}
	if start+availRows > total {
		start = total - availRows
	}
	if start < 0 {
		start = 0
	}
	end := start + availRows
	if end > total {
		end = total
	}
	visible := testCases[start:end]

	for i, tc := range visible {
		// Status Badge
		stBadge := yellowBold(tc.Status)
		if tc.Status == "complete" || tc.Status == "passed" || tc.Status == "active" {
			stBadge = greenBold(tc.Status)
		} else if tc.Status == "error" || tc.Status == "failed" {
			stBadge = redBold(tc.Status)
		}

		// Lineage Badge
		lineageBadge := greenBold("✓ INTACT")
		if tc.Lineage == nil || !tc.Lineage.IsIntact {
			lineageBadge = redBold("✗ BROKEN")
		}

		// Criteria progress
		crStr := fmt.Sprintf("%d/%d ok", tc.CompletedCriteria, tc.TotalCriteria)

		// Indicate row cursor if selected
		actualIdx := start + i
		idCell := tds.RowCursor(actualIdx == m.SelectedIndex, tc.ID)

		tcTable.AddRow(
			idCell,
			stBadge,
			lineageBadge,
			crStr,
			tc.Title,
		)
	}

	b.WriteString(tcTable.Render())

	// 3. Unbound Test Criteria notice if any exist
	if len(m.UnboundCriteria) > 0 {
		b.WriteString(tds.SectionDivider("⚠️ UNBOUND CRITERIA (Missing test_case binding)", w))
		critLimit := 2
		if len(m.UnboundCriteria) < critLimit {
			critLimit = len(m.UnboundCriteria)
		}
		for i := 0; i < critLimit; i++ {
			unc := m.UnboundCriteria[i]
			b.WriteString(fmt.Sprintf("  • %s  %s  %s\n",
				cyanBold(unc.ID), yellowBold("["+unc.Status+"]"), unc.Title))
		}
		b.WriteString("\n")
	}

	// 4. Recent Criteria Satisfied Ticker
	if len(m.RecentQAEvents) > 0 {
		b.WriteString(tds.SectionDivider("LIVE CRITERIA SATISFACTION TICKER", w))
		tickerLimit := 2
		if len(m.RecentQAEvents) < tickerLimit {
			tickerLimit = len(m.RecentQAEvents)
		}
		for i := 0; i < tickerLimit; i++ {
			ev := m.RecentQAEvents[i]
			tsStr := ev.Timestamp.Format("15:04:05")
			b.WriteString(fmt.Sprintf("  ⚡ [%s] %s\n", dim(tsStr), greenBold(ev.Message)))
		}
		b.WriteString("\n")
	}
}

// renderDetailModal renders an overlay modal dialog showing full record details.
func renderDetailModal(m *UIModel) string {
	w := m.Width
	if w < 70 {
		w = 80
	}
	h := m.Height
	if h < 20 {
		h = 24
	}

	modal := m.DetailModal
	if modal == nil {
		return ""
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("  %s %s │ %s %s",
		dim("OBJECT ID:"), whiteBold(modal.ID),
		dim("KIND:"), cyanBold(modal.Kind)))
	lines = append(lines, fmt.Sprintf("  %s %s │ %s %s",
		dim("STATUS   :"), greenBold(modal.Status),
		dim("TITLE:"), whiteBold(modal.Title)))
	if modal.Actor != "" {
		lines = append(lines, fmt.Sprintf("  %s %s", dim("ACTOR / OWNER :"), modal.Actor))
	}
	if modal.Timestamp != "" {
		lines = append(lines, fmt.Sprintf("  %s %s", dim("TIMESTAMP     :"), modal.Timestamp))
	}
	lines = append(lines, dim("  "+strings.Repeat("─", w-8)))

	if modal.Summary != "" {
		lines = append(lines, whiteBold("  DETAILED DIAGNOSTIC MESSAGE / SUMMARY:"))
		for _, sl := range strings.Split(modal.Summary, "\n") {
			slTrim := strings.TrimSpace(sl)
			if slTrim != "" {
				lines = append(lines, "    "+cyan(slTrim))
			}
		}
		lines = append(lines, "")
	}

	if len(modal.Details) > 0 {
		lines = append(lines, whiteBold("  PROPERTIES & METRICS:"))
		for _, d := range modal.Details {
			parts := strings.Split(d, "\n")
			for j, p := range parts {
				if j == 0 {
					lines = append(lines, "    "+p)
				} else {
					lines = append(lines, "      "+p)
				}
			}
		}
		lines = append(lines, "")
	}

	if len(modal.Lineage) > 0 {
		lines = append(lines, whiteBold("  TRACEABILITY & LINEAGE CHAIN:"))
		for _, l := range modal.Lineage {
			lines = append(lines, "    "+l)
		}
		lines = append(lines, "")
	}

	if len(modal.Criteria) > 0 {
		lines = append(lines, whiteBold("  BOUND CRITERIA & VERIFICATION:"))
		for _, c := range modal.Criteria {
			lines = append(lines, "    "+c)
		}
		lines = append(lines, "")
	}

	lines = append(lines, dim("  "+strings.Repeat("─", w-8)))
	if modal.Kind == objects.KindTestCase {
		lines = append(lines, dim("  [t] Re-scan QA Matrix │ [Esc]/[q] Close Modal"))
	} else if modal.Kind == objects.KindSchedulerJob {
		lines = append(lines, dim("  [t] Run Immediately │ [d] Run in 10s │ [Esc]/[q] Close Modal"))
	} else if modal.Kind == "correspondence" || modal.Kind == "tde_envelope" || modal.Kind == "inbox_item" {
		lines = append(lines, dim("  [a] Acknowledge Item │ [r] Quick Reply │ [Esc]/[q] Close Modal"))
	} else {
		lines = append(lines, dim("  Press [Esc] or [Backspace] or [q] to close modal and return to table view"))
	}
	lines = append(lines, "")

	// Pad or truncate to fit height
	maxLines := h - 4
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	panelTitle := fmt.Sprintf("🔍 DETAILED RECORD INSPECTION — %s", modal.ID)
	return tds.Panel(panelTitle, lines, w, tds.BorderHeavy)
}

// renderHealthTab renders the System Health & Action Center tab.
func renderHealthTab(b *strings.Builder, m *UIModel) {
	w := m.Width
	if w < 70 {
		w = 80
	}

	hs := m.HealthSummary

	// 1. Health & Integrity Vitals Card (TDS Panel)
	statusBadge := tds.Badge("HEALTHY")
	switch hs.OverallStatus {
	case "BLOCKED":
		statusBadge = tds.Badge("FAIL")
	case "ATTENTION":
		statusBadge = tds.Badge("WARN")
	case "NOTICE":
		statusBadge = tds.Badge("NOTICE")
	}

	freshBadge := tds.Badge("OK")
	if strings.HasPrefix(hs.CheckFreshness, "STALE") {
		freshBadge = tds.Badge("WARN")
	}

	overseerBadge := tds.Badge("ACTIVE")
	if !hs.OverseerRunning {
		overseerBadge = tds.Badge("STANDBY")
	}

	row1 := []tds.StatItem{
		{Label: "Kernel Integrity", Value: hs.OverallStatus, Extra: statusBadge},
		{Label: "Validation Cache", Value: hs.CheckFreshness, Extra: freshBadge},
		{Label: "Active Violations", Value: fmt.Sprintf("%d total", hs.TotalViolations), Extra: dim(fmt.Sprintf("(%d fixable)", hs.AutoFixableCount))},
	}
	row2 := []tds.StatItem{
		{Label: "Process Overseer", Value: fmt.Sprintf("%d/%d running", hs.DaemonsRunning, hs.DaemonsTotal), Extra: overseerBadge},
		{Label: "Storage Membrane", Value: fmt.Sprintf("%s (%d objs)", hs.StorageSizeStr, hs.StorageFiles)},
		{Label: "File Descriptors", Value: fmt.Sprintf("%d / %d", hs.OpenFileDesc, hs.MaxFileDesc), Extra: tds.Badge(ternary(hs.StaleLocksCount == 0, "CLEAN", "LOCKS"))},
	}

	var panelLines []string
	if w >= 125 {
		panelLines = []string{
			tds.StatRow(row1, w-4),
			tds.StatRow(row2, w-4),
		}
	} else {
		panelLines = []string{
			tds.StatRow(row1[:2], w-4),
			tds.StatRow([]tds.StatItem{row1[2], row2[0]}, w-4),
			tds.StatRow(row2[1:], w-4),
		}
	}
	b.WriteString(tds.Panel("Kernel Integrity Radar & System Health", panelLines, w, tds.BorderRounded))
	b.WriteString("\n")

	// 2. Process Group Overseer & Managed Daemons Section
	if len(m.DaemonHealth) > 0 {
		overseerHeader := "⚡ PROCESS GROUP OVERSEER & DAEMONS"
		if hs.OverseerRunning {
			overseerHeader += " [OVERSEER ACTIVE]"
		} else {
			overseerHeader += " [OVERSEER STANDBY]"
		}
		b.WriteString(tds.SectionDivider(overseerHeader, w))

		dTable := tds.NewTable(w).
			AddColumn("DAEMON", tds.AlignLeft, 14, 0.16).
			AddColumn("DESIRED", tds.AlignCenter, 10, 0.12).
			AddColumn("ACTUAL", tds.AlignCenter, 10, 0.12).
			AddColumn("PID", tds.AlignCenter, 8, 0.10).
			AddColumn("PGID", tds.AlignCenter, 8, 0.10).
			AddColumn("RESTARTS", tds.AlignCenter, 10, 0.12).
			AddColumn("UPTIME", tds.AlignCenter, 12, 0.14).
			AddColumn("STATUS", tds.AlignCenter, 10, 0.14)

		for _, d := range m.DaemonHealth {
			stBadge := dim(d.Status)
			switch d.Status {
			case "HEALTHY", "RUNNING":
				stBadge = greenBold(d.Status)
			case "BACKOFF":
				stBadge = yellowBold(d.Status)
			case "CRASHED", "FAIL":
				stBadge = redBold(d.Status)
			}

			pidStr := "--"
			if d.PID > 0 {
				pidStr = fmt.Sprintf("%d", d.PID)
			}
			pgidStr := "--"
			if d.PGID > 0 {
				pgidStr = fmt.Sprintf("%d", d.PGID)
			}

			dTable.AddRow(
				cyanBold(d.Name),
				d.DesiredState,
				d.ActualState,
				pidStr,
				pgidStr,
				fmt.Sprintf("%d", d.RestartCount),
				d.Uptime,
				stBadge,
			)
		}
		b.WriteString(dTable.Render())
		b.WriteString("\n")
	}

	// 3. Action Center Card (Tray-Backed Scheduler Triggers)
	if len(m.ActionItems) > 0 {
		b.WriteString(tds.SectionDivider("⚡ ACTION CENTER (Press key to trigger background scheduler job)", w))
		var actionLines []string
		for _, item := range m.ActionItems {
			keyBadge := whiteBold(fmt.Sprintf("[%s]", strings.ToUpper(item.Key)))
			nameStr := cyanBold(item.Name)
			stStr := dim(item.JobID)
			if item.IsTriggered {
				switch item.Status {
				case "completed", "processed":
					stStr = greenBold("✓ PROCESSED")
				case "processing":
					stStr = yellowBold("⚡ PROCESSING")
				case "failed":
					stStr = redBold("✗ FAILED")
				default:
					stStr = cyanBold("⏳ ENQUEUED")
				}
			}
			actionLines = append(actionLines, fmt.Sprintf("  %s %-24s %-40s %s", keyBadge, nameStr, item.Description, stStr))
		}
		// Print top 4-6 actions
		limit := 4
		if m.Height >= 40 {
			limit = 6
		}
		if len(actionLines) < limit {
			limit = len(actionLines)
		}
		for i := 0; i < limit; i++ {
			b.WriteString(actionLines[i] + "\n")
		}
		b.WriteString("\n")
	}

	// 3. Active Violations & Integrity Warnings Table
	violations := m.GetVisibleHealthViolations()
	if len(violations) == 0 {
		b.WriteString(tds.SectionDivider("ACTIVE VIOLATIONS & INTEGRITY WARNINGS", w))
		if m.SearchQuery != "" {
			b.WriteString(dim(fmt.Sprintf("  [No integrity violations matching search query \"%s\"]\n\n", m.SearchQuery)))
		} else {
			b.WriteString(dim("  [No integrity violations found in validation cache — kernel is clean]\n\n"))
		}
		return
	}

	b.WriteString(tds.SectionDivider("ACTIVE VIOLATIONS & INTEGRITY WARNINGS (Press Enter to inspect)", w))
	vTable := tds.NewTable(w).
		AddColumn("TIER", tds.AlignCenter, 6, 0.08).
		AddColumn("SEVERITY", tds.AlignCenter, 10, 0.12).
		AddColumn("KIND", tds.AlignLeft, 14, 0.16).
		AddColumn("OBJECT ID", tds.AlignLeft, 16, 0.20).
		AddColumn("MESSAGE", tds.AlignLeft, 30, 0.44)

	overhead := 21
	if len(m.DaemonHealth) > 0 {
		overhead += len(m.DaemonHealth) + 4
	}
	if len(m.ActionItems) > 0 {
		overhead += 5
	}
	availRows := m.Height - overhead + m.ProfileSpacingBonus()
	if availRows < 3 {
		availRows = 3
	}

	total := len(violations)
	start := 0
	if m.SelectedIndex >= availRows {
		start = m.SelectedIndex - availRows + 1
	}
	if start+availRows > total {
		start = total - availRows
	}
	if start < 0 {
		start = 0
	}
	end := start + availRows
	if end > total {
		end = total
	}
	visible := violations[start:end]

	for i, v := range visible {
		tierStr := fmt.Sprintf("T%d", v.Tier)
		sevBadge := dim(v.Severity)
		switch v.Tier {
		case 1:
			sevBadge = redBold(v.Severity)
			tierStr = redBold(tierStr)
		case 2:
			sevBadge = yellowBold(v.Severity)
			tierStr = yellowBold(tierStr)
		case 3:
			sevBadge = cyanBold(v.Severity)
			tierStr = cyanBold(tierStr)
		}

		actualIdx := start + i
		idCell := tds.RowCursor(actualIdx == m.SelectedIndex, v.ObjectID)

		vTable.AddRow(
			tierStr,
			sevBadge,
			v.Kind,
			idCell,
			v.Message,
		)
	}

	b.WriteString(vTable.Render())
	b.WriteString("\n")
}

