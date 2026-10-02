package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewWstransShowCmd creates the wstrans show subcommand.
func NewWstransShowCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectWstransShowCommandBuilder()
	cli.BindAsyncProgress(cmd, runWstransShow)
	return cmd
}

func runWstransShow(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		return showSingletonOrFirstObject(cmd, proc, args, objects.KindWorkstreamTransition, "workstream transition", "workstream transitions")
	})(cmd, args)
}
