package object

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// NewSPlanListCmd creates the splan list subcommand.
// Generated spec: docs/process/command_specs/object/splan/list_command.yaml; query flags from generated builder (query_flags: true).
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
