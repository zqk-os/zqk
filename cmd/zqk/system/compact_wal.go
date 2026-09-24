package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// NewCompactWALCmd creates a command to compact the WAL file
func NewCompactWALCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemCompactWalCommandBuilder()
	cmd.RunE = runCompactWAL
	return cmd
}

func runCompactWAL(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		projectRoot := proc.ProjectRoot()
		if projectRoot == "" {
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

		output := map[string]any{
			"success":      true,
			"project_root": projectRoot,
			"message":      "WAL compaction completed successfully",
		}
		return cli.FormatOutput(cmd, output)
	})(cmd, args)
}
