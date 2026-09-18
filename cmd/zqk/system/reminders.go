package system

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewRemindersCmd creates a reminders command
func NewRemindersCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Show active reminders and notifications",
		"Show active reminders and notifications that require attention.",
		"",
		"This command displays:",
		"  - CLI-first commitment reminders",
		"  - System check reminders",
		"  - Other active notifications",
	).
		AddExample("Show all reminders", "%s system reminders").
		AddExample("Show only CLI reminders", "%s system reminders --type cli").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRemindersCommandBuilder(), &cobra.Command{
		Use: "reminders",
	})
	cli.BindAsyncProgress(cmd, func(cmd *cobra.Command, args []string) error {
		ctx := cli.GetContext(cmd)
		if ctx == nil {
			return errfmt.Errorf("failed to get context")
		}
		return showReminders(ctx, cmd)
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("type", "", "Filter by reminder type (cli, system-check, all)")
	cli.AddCommonFlags(cmd)
	return cmd
}

func showReminders(ctx *cli.Context, cmd *cobra.Command) error {
	projectRoot := ctx.ProjectRoot
	projectRoot = ProjectRootOrResolve(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	reminderType, _ := cmd.Flags().GetString("type")
	hasReminders := false
	var b strings.Builder

	if reminderType == emptyValue || reminderType == "cli" || reminderType == "all" {
		flagFile := filepath.Join(projectRoot, paths.ProjectDataDir, "cli_reminder.flag")
		if info, err := fileutil.Stat(flagFile); err == nil {
			hasReminders = true
			b.WriteString("🚨 CLI-FIRST REMINDER:\n")
			b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			if content, err := fileutil.ReadFile(flagFile); err == nil {
				_, _ = b.Write(content)
				b.WriteByte('\n')
			}

			modTime := info.ModTime()
			age := time.Since(modTime)
			_, _ = fmt.Fprintf(&b, "\n⏰ Last updated: %s (%s ago)\n", modTime.Format("2006-01-02 15:04:05"), formatReminderDuration(age))
			b.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			b.WriteString("\n")
		}
	}

	if !hasReminders {
		b.WriteString("✅ No active reminders\n")
		return cli.WriteOutput(cmd, []byte(b.String()))
	}

	return cli.WriteOutput(cmd, []byte(b.String()))
}

func formatReminderDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// checkCLIReminder checks for CLI reminder and displays it if present (routes via cli.WriteOutput).
// Called from system status.
func checkCLIReminder(cmd *cobra.Command, projectRoot string) bool {
	if projectRoot == emptyValue || cmd == nil {
		return false
	}

	flagFile := filepath.Join(projectRoot, paths.ProjectDataDir, "cli_reminder.flag")
	if info, err := fileutil.Stat(flagFile); err == nil {
		var b strings.Builder
		b.WriteString("\n")
		b.WriteString("🚨 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		b.WriteString("🚨 CLI-FIRST REMINDER (INTERRUPT NOTIFICATION)\n")
		b.WriteString("🚨 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		if content, err := fileutil.ReadFile(flagFile); err == nil {
			_, _ = b.Write(content)
			b.WriteByte('\n')
		}

		modTime := info.ModTime()
		age := time.Since(modTime)
		_, _ = fmt.Fprintf(&b, "⏰ Last updated: %s (%s ago)\n", modTime.Format("2006-01-02 15:04:05"), formatDuration(age))
		b.WriteString("🚨 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		b.WriteString("\n")
		_ = cli.WriteOutput(cmd, []byte(b.String()))
		return true
	}
	return false
}
