package ambient

import (
	"github.com/zqk-os/zqk/pkg/ambient"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

func newAutomergeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAmbientAutomergeCommandBuilder()
	cmd.RunE = runAmbientAutomerge
	return cmd
}

func runAmbientAutomerge(cmd *cobra.Command, _ []string) error {
	eventLogger := logging.GetLoggerFromContext(cmd.Context())
	daemon := ambient.NewAutoMergeDaemon(eventLogger.Logger())
	daemon.Start(cmd.Context())
	return nil
}
