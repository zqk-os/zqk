package feed

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewFeedCmd creates the feed command group (steer / emit-status / proof-of-life / ack / pending / doctor / bridge-ingest / serve / watch / wake).
func NewFeedCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedCommandBuilder()
	cmd.AddCommand(NewSteerCmd())
	cmd.AddCommand(NewEmitStatusCmd())
	cmd.AddCommand(NewProofOfLifeCmd())
	cmd.AddCommand(NewAckCmd())
	cmd.AddCommand(NewPendingCmd())
	cmd.AddCommand(NewDoctorCmd())
	cmd.AddCommand(NewBridgeIngestCmd())
	cmd.AddCommand(NewServeCmd())
	cmd.AddCommand(NewWatchCmd())
	cmd.AddCommand(NewWakeCmd())
	return cmd
}
