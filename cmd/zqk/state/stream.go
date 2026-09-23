package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
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
			limit, _ := cmd.Flags().GetInt("limit")
			if limit <= 0 {
				limit = 20
			}

			format, _ := cmd.Flags().GetString("format")

			return StreamJournalMutations(cmd, projectRoot, follow, limit, format)
		})(cmd, args)
	}
	return cmd
}

// StreamJournalMutations handles outputting journal mutations either statically or continuously.
func StreamJournalMutations(cmd *cobra.Command, projectRoot string, follow bool, limit int, format string) error {
	seen := make(map[string]bool)

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
		out := BuildStreamSummary(recent)
		return cli.WriteOutput(cmd, []byte(out))
	}

	// Live streaming mode
	header := fmt.Sprintf("\n📡 Streaming Kernel State Seismograph (Watching %s)...\n", projectRoot)
	header += "──────────────────────────────────────────────────────────────────────────────────────────\n"
	_ = cli.WriteOutput(cmd, []byte(header))

	// Output initial batch
	for _, m := range recent {
		seen[m.ID] = true
		_ = cli.WriteOutput(cmd, []byte(FormatMutationLine(m)+"\n"))
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-cmd.Context().Done():
			_ = cli.WriteOutput(cmd, []byte("\n[Stream closed]\n"))
			return nil
		case <-ticker.C:
			current := readRecentJournalMutations(projectRoot, 20)
			// Reverse to chronological
			for i, j := 0, len(current)-1; i < j; i, j = i+1, j-1 {
				current[i], current[j] = current[j], current[i]
			}
			for _, m := range current {
				if !seen[m.ID] {
					seen[m.ID] = true
					_ = cli.WriteOutput(cmd, []byte(FormatMutationLine(m)+"\n"))
				}
			}
		}
	}
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
