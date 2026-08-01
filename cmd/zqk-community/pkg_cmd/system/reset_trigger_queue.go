package system

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/spf13/cobra"
)

func NewResetTriggerQueueCmd() *cobra.Command {
	return clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemResetTriggerQueueCommandBuilder(), &cobra.Command{
		Use:   "reset-trigger-queue",
		Short: "Clear the scheduler trigger queue to prevent startup DDOS",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get path using internal paths logic
			queueDir := filepath.Join(os.Getenv(zqkenv.ProjectRoot()), paths.ProjectDataDir, "scheduler", "triggers")
			err := os.RemoveAll(queueDir)
			if err == nil {
				return cli.WriteOutput(cmd, []byte("Trigger queue cleared.\n"))
			}
			return err
		},
	})
}
