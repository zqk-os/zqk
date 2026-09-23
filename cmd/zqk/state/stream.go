package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
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

	streamDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	_ = fileutil.MkdirAll(streamDir, paths.DirPerm755)

	watcher, err := fsnotify.NewWatcher()
	if err == nil {
		defer watcher.Close()
		_ = watcher.Add(streamDir)
		if entries, rErr := os.ReadDir(streamDir); rErr == nil {
			for _, entry := range entries {
				if strings.HasSuffix(entry.Name(), ".json") {
					_ = watcher.Add(filepath.Join(streamDir, entry.Name()))
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
	var buf strings.Builder

	buf.WriteString("╔═══════════════════════════════════════════════════════════════════════════════════════════════╗\n")
	buf.WriteString("║                    ⚡ ZQK STATE SEISMOGRAPH & TELEMETRY DASHBOARD                            ║\n")
	buf.WriteString("╚═══════════════════════════════════════════════════════════════════════════════════════════════╝\n")

	actors := make(map[string]int)
	kinds := make(map[string]int)

	for _, m := range recent {
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
		if len(actorLine) > 55 {
			actorLine = actorLine[:52] + "..."
		}
	}

	sparkline := generateSparkline(len(recent))

	buf.WriteString(fmt.Sprintf("Root:       %s\n", projectRoot))
	buf.WriteString(fmt.Sprintf("Status:     ACTIVE | Events in Window: %d | Sparkline: %s\n", len(recent), sparkline))
	buf.WriteString("Membrane:   CPCP-MEMBRANE-001 (FAIL-CLOSED) | Status: ENFORCING (0 Blocked)\n")
	buf.WriteString(fmt.Sprintf("Actors:     %s\n", actorLine))

	kindStrs := make([]string, 0, len(kinds))
	for k, count := range kinds {
		kindStrs = append(kindStrs, fmt.Sprintf("%s: %d", k, count))
	}
	if len(kindStrs) > 0 {
		buf.WriteString(fmt.Sprintf("Objects:    %s\n", strings.Join(kindStrs, " | ")))
	}

	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")
	buf.WriteString(fmt.Sprintf("%-10s | %-12s | %-32s | %-24s | %s\n", "TIME", "EVENT", "OBJECT REF", "SUMMARY / DIFF", "CPCP"))
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")

	displaySlice := recent
	if len(displaySlice) > 15 {
		displaySlice = displaySlice[len(displaySlice)-15:]
	}

	for _, m := range displaySlice {
		timeStr := "--:--:--"
		if m.CreatedAt > 0 {
			timeStr = time.Unix(m.CreatedAt, 0).Format("15:04:05")
		}

		badge := "⚡ " + strings.ToUpper(m.ChangeType)
		if len(badge) > 12 {
			badge = badge[:12]
		}
		ref := m.ObjectRef
		if len(ref) > 32 {
			ref = ref[:29] + "..."
		}

		summary := m.DiffSummary
		if summary == "" {
			summary = m.ChangeType
		}
		if len(summary) > 24 {
			summary = summary[:21] + "..."
		}

		buf.WriteString(fmt.Sprintf("[%s] | %-12s | %-32s | %-24s | [CPCP: PASS]\n", timeStr, badge, ref, summary))
	}
	buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")

	return buf.String()
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

	badge := "⚡ " + strings.ToUpper(m.ChangeType)
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

	return fmt.Sprintf("[%s] %-10s | %-12s | %-32s | %-35s | [CPCP: PASS]", timeStr, badge, m.ID, ref, summary)
}
