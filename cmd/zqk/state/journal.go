package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

func newJournalCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStateJournalCommandBuilder()
	cmd.Aliases = []string{"log", "history"}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
			projectRoot := proc.ProjectRoot()
			if projectRoot == "" {
				projectRoot = cli.ResolveProjectRoot(".")
			}

			mutations := readRecentJournalMutations(projectRoot, limit)

			format, _ := cmd.Flags().GetString("format")
			if format == "json" {
				return cli.FormatOutputAs(cmd, cli.FormatJSON, map[string]any{
					"count":     len(mutations),
					"mutations": mutations,
				})
			}

			var buf strings.Builder
			buf.WriteString(fmt.Sprintf("\n📜 Recent Change Journal Mutations (Total: %d)\n", len(mutations)))
			buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")
			buf.WriteString(fmt.Sprintf("%-12s %-8s %-36s %s\n", "ID", "TYPE", "OBJECT REF", "DIFF / SUMMARY"))
			buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n")

			for _, m := range mutations {
				diff := m.DiffSummary
				if diff == "" {
					diff = m.ChangeType
				}
				if len(diff) > 40 {
					diff = diff[:37] + "..."
				}
				timeStr := ""
				if m.CreatedAt > 0 {
					t := time.Unix(m.CreatedAt, 0)
					timeStr = t.Format("15:04:05")
				}
				ref := m.ObjectRef
				if len(ref) > 35 {
					ref = ref[:32] + "..."
				}
				buf.WriteString(fmt.Sprintf("%-12s %-8s %-36s %s (%s)\n", m.ID, m.ChangeType, ref, diff, timeStr))
			}
			buf.WriteString("──────────────────────────────────────────────────────────────────────────────────────────\n\n")

			return cli.WriteOutput(cmd, []byte(buf.String()))
		})(cmd, args)
	}
	return cmd
}
