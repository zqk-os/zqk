package object

import (
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewSPlanCmd creates the strategic plan command group (BLI-805).
// Uses generated builder from .zqk/cli/specs/object/splan_command.yaml; subcommands add RunE and query flags.
func NewSPlanCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectSplanCommandBuilder()
	cmd.AddCommand(NewSPlanShowCmd())
	cmd.AddCommand(NewSPlanListCmd())
	return cmd
}
