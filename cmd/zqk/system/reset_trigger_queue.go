package system

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func NewResetTriggerQueueCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemResetTriggerQueueCommandBuilder(), &cobra.Command{
		Use:   "reset-trigger-queue",
		Short: "Clear the scheduler trigger queue to prevent startup DDOS",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get path using internal paths logic
			queueDir := filepath.Join(zqkenv.ProjectRoot().Get(), paths.ProjectDataDir, paths.SchedulerSubdir, "triggers")
			err := fileutil.RemoveAll(queueDir)
			if err == nil {
				return cli.WriteOutput(cmd, []byte("Trigger queue cleared.\n"))
			}
			return err
		},
	})
}
