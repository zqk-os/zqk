package system

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
)

func NewRestartCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemRestartCommandBuilder(), &cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			logger := logging.GetLogger()
			logging.FluentEvent(logger).Info("Initiating system daemon restart...").Log()

			// Shutdown system daemons cleanly
			shutdownCmd := NewShutdownCmd()
			shutdownCmd.Run(shutdownCmd, args)

			// Start system daemons
			startCmd := NewStartCmd()
			startCmd.Run(startCmd, args)
		},
	})
	return cmd
}
