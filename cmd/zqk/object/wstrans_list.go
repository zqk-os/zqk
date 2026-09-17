package object

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewWstransListCmd creates the wstrans list subcommand (BLI-807).
// Generated spec: .zqk/cli/specs/object/wstrans/list_command.yaml; query flags from generated builder (query_flags: true).
func NewWstransListCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectWstransListCommandBuilder()
	cli.BindAsyncProgress(cmd, runWstransList)
	return cmd
}

func runWstransList(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err
		return runListSingleKind(cmd, proc, objects.KindWorkstreamTransition)
	})(cmd, nil)
}
