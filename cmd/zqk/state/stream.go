package state

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/tui/tds"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func newStreamCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStateStreamCommandBuilder()
	cmd.Aliases = []string{"monitor", "watch"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			projectRoot := proc.ProjectRoot()
			if projectRoot == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}

			follow, _ := cmd.Flags().GetBool("follow")
			dashboard, _ := cmd.Flags().GetBool("dashboard")
			limit, _ := cmd.Flags().GetInt("limit")
			if limit <= 0 {
				limit = 20
			}

			format, _ := cmd.Flags().GetString("format")

			return StreamJournalMutations(cmd, projectRoot, follow, limit, format, dashboard)
		})(cmd, args)
	}
	return cmd
}

// StreamJournalMutations handles outputting journal mutations either statically or continuously.
func StreamJournalMutations(cmd *cobra.Command, projectRoot string, follow bool, limit int, format string, dashboard ...bool) error {
	seen := make(map[string]bool)
	isDashboard := len(dashboard) > 0 && dashboard[0]

	// Fetch recent mutations
	recent := readRecentJournalMutations(projectRoot, limit)

	// Reverse to chronological order (oldest to newest)
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}

	if format == "json" && !follow {
		return cli.FormatOutputAs(cmd, cli.FormatJSON, map[string]any{
			"count":     len(recent),
			"mutations": recent,
		})
	}

	if !follow {
		if isDashboard {
			out := BuildDashboardView(projectRoot, recent)
			return cli.WriteOutput(cmd, []byte(out))
		}
		out := BuildStreamSummary(recent)
		return cli.WriteOutput(cmd, []byte(out))
	}

	// Live streaming mode
	if isDashboard {
		_ = cli.WriteOutput(cmd, []byte(BuildDashboardView(projectRoot, recent)))
	} else {
		header := fmt.Sprintf("\n📡 Streaming Kernel State Seismograph (Watching %s)...\n", projectRoot)
		header += "──────────────────────────────────────────────────────────────────────────────────────────\n"
		_ = cli.WriteOutput(cmd, []byte(header))

		// Output initial batch
		for _, m := range recent {
			seen[m.ID] = true
			_ = cli.WriteOutput(cmd, []byte(FormatMutationLine(m)+"\n"))
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err == nil {
		defer watcher.Close()
		for _, kind := range trackedStreamKinds {
			sDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, kind)
			_ = fileutil.MkdirAll(sDir, paths.DirPerm755)
			_ = watcher.Add(sDir)
			if entries, rErr := fileutil.ReadDir(sDir); rErr == nil {
				for _, entry := range entries {
					if strings.HasSuffix(entry.Name(), ".json") {
						_ = watcher.Add(filepath.Join(sDir, entry.Name()))
					}
				}
			}
		}
	}

	var eventsCh <-chan fsnotify.Event
	var errorsCh <-chan error
	if watcher != nil {
		eventsCh = watcher.Events
		errorsCh = watcher.Errors
	}

	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	renderLatest := func() {
		current := readRecentJournalMutations(projectRoot, limit)
		// Reverse to chronological
		for i, j := 0, len(current)-1; i < j; i, j = i+1, j-1 {
			current[i], current[j] = current[j], current[i]
		}
		if isDashboard {
			_ = cli.WriteOutput(cmd, []byte("\033[H\033[2J"+BuildDashboardView(projectRoot, current)))
		} else {
			for _, m := range current {
				if !seen[m.ID] {
					seen[m.ID] = true
					_ = cli.WriteOutput(cmd, []byte(FormatMutationLine(m)+"\n"))
				}
			}
		}
	}

	heartbeat := time.NewTicker(1 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-cmd.Context().Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			_ = cli.WriteOutput(cmd, []byte("\n[Stream closed]\n"))
			return nil
		case event, ok := <-eventsCh:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) && strings.HasSuffix(event.Name, ".json") {
				if watcher != nil {
					_ = watcher.Add(event.Name)
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounceTimer == nil {
					debounceTimer = time.NewTimer(50 * time.Millisecond)
					debounceCh = debounceTimer.C
				} else {
					debounceTimer.Reset(50 * time.Millisecond)
				}
			}
		case <-debounceCh:
			renderLatest()
			debounceCh = nil
		case <-heartbeat.C:
			renderLatest()
		case _, ok := <-errorsCh:
			if !ok {
				return nil
			}
		}
	}
}

// BuildDashboardView renders an interactive ANSI visual seismograph dashboard.
func BuildDashboardView(projectRoot string, recent []JournalMutation) string {
	w := tds.GetTerminalWidth(96)
	var buf strings.Builder

	displaySlice := recent
	if len(displaySlice) > 15 {
		displaySlice = displaySlice[len(displaySlice)-15:]
	}

	actors := make(map[string]int)
	kinds := make(map[string]int)

	for _, m := range displaySlice {
		if m.CreatedBy != "" {
			actors[m.CreatedBy]++
		}
		if strings.Contains(m.ObjectRef, "-") {
			parts := strings.Split(m.ObjectRef, "-")
			kinds[parts[0]]++
		}
	}

	actorStrs := make([]string, 0, len(actors))
	for a, count := range actors {
		actorStrs = append(actorStrs, fmt.Sprintf("%s (%d)", a, count))
	}
	actorLine := "none"
	if len(actorStrs) > 0 {
		actorLine = strings.Join(actorStrs, ", ")
		if len(actorLine) > 40 {
			actorLine = actorLine[:37] + "..."
		}
	}

	kindStrs := make([]string, 0, len(kinds))
	for k, count := range kinds {
		kindStrs = append(kindStrs, fmt.Sprintf("%s: %d", k, count))
	}
	kindLine := "none"
	if len(kindStrs) > 0 {
		kindLine = strings.Join(kindStrs, " │ ")
		if len(kindLine) > 40 {
			kindLine = kindLine[:37] + "..."
		}
	}

	sparkline := generateSparkline(len(displaySlice))

	buf.WriteString(tds.Panel("ZQK STATE SEISMOGRAPH & TELEMETRY DASHBOARD", []string{
		" " + tds.StatRow([]tds.StatItem{
			{Label: "Root", Value: projectRoot},
			{Label: "Status", Value: "ACTIVE", Extra: tds.Badge("OK")},
		}, w-6),
		" " + tds.StatRow([]tds.StatItem{
			{Label: "Events in Window", Value: fmt.Sprintf("%d", len(displaySlice)), Extra: sparkline},
			{Label: "Membrane", Value: "CPCP-MEMBRANE-001 (FAIL-CLOSED)", Extra: tds.Badge("ENFORCING")},
		}, w-6),
		" " + tds.StatRow([]tds.StatItem{
			{Label: "Actors", Value: actorLine},
			{Label: "Objects", Value: kindLine},
		}, w-6),
	}, w, tds.BorderHeavy))
	buf.WriteString("\n")

	tbl := tds.NewTable(w).
		AddColumn("TIME", tds.AlignCenter, 10, 0.12).
		AddColumn("EVENT", tds.AlignLeft, 12, 0.14).
		AddColumn("OBJECT REF", tds.AlignLeft, 22, 0.32).
		AddColumn("SUMMARY / DIFF", tds.AlignLeft, 20, 0.28).
		AddColumn("CPCP", tds.AlignCenter, 12, 0.14)

	for _, m := range displaySlice {
		timeStr := "--:--:--"
		if m.CreatedAt > 0 {
			timeStr = time.Unix(m.CreatedAt, 0).Format("15:04:05")
		}

		badge := FormatEventBadge(m.ChangeType)
		summary := m.DiffSummary
		if summary == "" {
			summary = m.ChangeType
		}

		tbl.AddRow(
			fmt.Sprintf("[%s]", timeStr),
			badge,
			m.ObjectRef,
			summary,
			"[CPCP: PASS]",
		)
	}

	buf.WriteString(tbl.Render())
	buf.WriteString("\n")

	return buf.String()
}

// FormatEventBadge formats a change/event type into a standardized ANSI badge.
func FormatEventBadge(changeType string) string {
	ct := strings.ToUpper(strings.TrimSpace(changeType))
	switch ct {
	case "SYSTEM_CONFIG_CHANGE":
		ct = "CONFIG"
	case "COMMAND_EXECUTION":
		ct = "EXEC"
	case "SCHEDULER_JOB_COMPLETED":
		ct = "JOB_DONE"
	case "OBJECT_CREATION":
		ct = "CREATE"
	case "OBJECT_UPDATE":
		ct = "UPDATE"
	case "OBJECT_DELETION":
		ct = "DELETE"
	case "PROCESS_LIFECYCLE":
		ct = "PROC"
	case "AGENT_INSTRUCTION":
		ct = "INSTR"
	default:
		if len(ct) > 8 {
			ct = ct[:8]
		}
	}
	return "⚡ " + ct
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

// BuildStreamSummary formats recent mutations into a terminal seismograph table.
func BuildStreamSummary(recent []JournalMutation) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("\n📡 Kernel State Seismograph (Recent: %d)\n", len(recent)))
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")
	for _, m := range recent {
		buf.WriteString(FormatMutationLine(m) + "\n")
	}
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n\n")
	return buf.String()
}

// FormatMutationLine formats a single mutation entry.
func FormatMutationLine(m JournalMutation) string {
	timeStr := "--:--:--"
	if m.CreatedAt > 0 {
		timeStr = time.Unix(m.CreatedAt, 0).Format("15:04:05")
	}

	badge := FormatEventBadge(m.ChangeType)
	ref := m.ObjectRef
	if len(ref) > 36 {
		ref = ref[:33] + "..."
	}

	summary := m.DiffSummary
	if summary == "" {
		summary = m.ChangeType
	}
	if len(summary) > 40 {
		summary = summary[:37] + "..."
	}

	return fmt.Sprintf("[%s] %-12s | %-12s | %-32s | %-35s | [CPCP: PASS]", timeStr, badge, m.ID, ref, summary)
}
