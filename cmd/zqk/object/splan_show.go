package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewSPlanShowCmd creates the splan show subcommand.
func NewSPlanShowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectSplanShowCommandBuilder()
	cli.BindAsyncProgress(cmd, runSPlanShow)
	return cmd
}

func runSPlanShow(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		return showSingletonOrFirstObject(cmd, proc, args, objects.KindStrategicPlan, "strategic plan", "strategic plans")
	})(cmd, args)
}
