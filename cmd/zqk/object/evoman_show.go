package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewEvomanShowCmd creates the evoman show subcommand.
func NewEvomanShowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectEvomanShowCommandBuilder()
	cli.BindAsyncProgress(cmd, runEvomanShow)
	return cmd
}

func runEvomanShow(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		return showSingletonOrFirstObject(cmd, proc, args, objects.KindEvolutionManagement, "evolution management", "evolution management objects")
	})(cmd, args)
}
