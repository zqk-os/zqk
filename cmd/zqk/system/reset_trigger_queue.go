package system

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func NewResetTriggerQueueCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemResetTriggerQueueCommandBuilder(), &cobra.Command{
		Use:   "reset-trigger-queue",
		Short: "Clear the scheduler trigger queue to prevent startup DDOS",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get path using internal paths logic
			queueDir := filepath.Join(zqkenv.ProjectRoot().Get(), paths.ProjectDataDir, "scheduler", "triggers")
			err := fileutil.RemoveAll(queueDir)
			if err == nil {
				return cli.WriteOutput(cmd, []byte("Trigger queue cleared.\n"))
			}
			return err
		},
	})
}
