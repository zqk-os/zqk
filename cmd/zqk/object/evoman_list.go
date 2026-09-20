package object

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewEvomanListCmd creates the evoman list subcommand (BLI-808).
// Generated spec: .zqk/cli/specs/object/evoman/list_command.yaml; query flags from generated builder (query_flags: true).
func NewEvomanListCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectEvomanListCommandBuilder()
	cli.BindAsyncProgress(cmd, runEvomanList)
	return cmd
}

func runEvomanList(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err
		return runListSingleKind(cmd, proc, objects.KindEvolutionManagement)
	})(cmd, nil)
}
