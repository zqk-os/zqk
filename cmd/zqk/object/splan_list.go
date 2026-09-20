package object

import (
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewSPlanListCmd creates the splan list subcommand.
// Generated spec: .zqk/cli/specs/object/splan/list_command.yaml; query flags from generated builder (query_flags: true).
func NewSPlanListCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectSplanListCommandBuilder()
	cli.BindAsyncProgress(cmd, runSPlanList)
	return cmd
}

func runSPlanList(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var err error
		_ = err
		return runListSingleKind(cmd, proc, objects.KindStrategicPlan)
	})(cmd, args)
}
