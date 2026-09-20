package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// NewCompactWALCmd creates a command to compact the WAL file
func NewCompactWALCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Compact the WAL file by removing applied entries",
		"Removes entries from the WAL file that have already been applied (based on checkpoint).",
		"",
		"This reduces WAL file size by keeping only unapplied entries.",
		"When a scheduler daemon holds the object WAL open, standalone compaction can orphan file descriptors or corrupt state — this command refuses unless you pass --force.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCompactWalCommandBuilder(), &cobra.Command{
		Use:   "compact-wal",
		Short: "Compact WAL file by removing applied entries",
		RunE:  runCompactWAL,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().Bool("force", false, "Allow compaction while the scheduler daemon appears to be running (unsafe; prefer stopping the daemon or letting the write-behind worker compact via TryCompactWAL)")

	return cmd
}

func runCompactWAL(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}

	projectRoot := ProjectRootOrResolve("")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	force, _ := cmd.Flags().GetBool("force")
	if !force {
		running, pid, runErr := schedulerpkg.IsSchedulerRunning(projectRoot)
		if runErr == nil && running {
			return errfmt.Errorf("scheduler daemon appears to be running (PID %d): standalone object WAL compaction can conflict with the open WAL in that process — stop the scheduler first, or pass --force if you accept the risk (see docs/architecture/OBJECT_WAL_FD_LIFECYCLE.md)", pid)
		}
	}

	// Compact the WAL
	if err := storagepkg.CompactWAL(projectRoot); err != nil {
		return errfmt.Newf("failed to compact WAL").Wrap(err)
	}

	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		output := map[string]any{
			"success":      true,
			"project_root": projectRoot,
			"message":      "WAL compaction completed successfully",
		}
		return cli.FormatOutput(cmd, output)
	}

	// POL-CODE-007: user-facing output via logger
	logger := logging.GetLoggerFromProfile(ctx.Profile)
	logging.Fluent(logger).Info("WAL compaction completed successfully").
		ProjectRoot(projectRoot).
		Log()
	return nil
}
