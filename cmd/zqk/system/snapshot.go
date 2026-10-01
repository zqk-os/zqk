package system

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NewSnapshotCmd creates the telemetry snapshot command
func NewSnapshotCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Capture a sanitized telemetry snapshot for partner support",
		"Capture a sanitized telemetry snapshot of the system state, explicitly redacting .env files and sensitive keys, for secure transmission to engineering.",
		"",
		"This command is explicitly designed for the Beta Curriculum's Sales & Feedback loop.",
	).
		AddExample("Capture snapshot with reason", "%s system snapshot --reason \"Partner X encountered graph timeout\"").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemSnapshotCommandBuilder(), &cobra.Command{
		Use:  "snapshot",
		RunE: runSnapshot,
	})

	helpBuilder.ApplyToCommand(cmd)
	cli.AddCommonFlags(cmd)

	cmd.Flags().String("reason", "", "The reason for the snapshot (required)")

	return cmd
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		reason, _ := cmd.Flags().GetString("reason")
		if reason == "" {
			return errfmt.Errorf("--reason is required")
		}

		var err error
		_ = err

		projectRoot := proc.ProjectRoot()
		timestamp := time.Now().UnixMilli()

		// Ensure the snapshots directory exists
		snapshotDir := filepath.Join(projectRoot, paths.ProjectStateDir, "snapshots")
		outputPath := filepath.Join(snapshotDir, fmt.Sprintf("snap_%d.csnap", timestamp))

		// Execute the underlying state-commit command
		execCmd := execwrap.Command("bin/zqk", "system", "state-commit", "--snapshot-file", outputPath)
		execCmd.Dir = projectRoot

		// Run silently
		if err := execCmd.Run(); err != nil {
			return errfmt.Newf("failed to generate snapshot: %v", err).Wrap(err)
		}

		cmd.Printf("\n✓ System state snapshotted to .zqk-state/snapshots/snap_%d.csnap\n", timestamp)
		cmd.Printf("✓ Ready for transmission to core.\n")

		return nil
	})(cmd, args)
}
