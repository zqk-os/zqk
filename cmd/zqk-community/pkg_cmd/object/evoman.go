package object

import (
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewEvomanCmd creates the evolution management command group (ITEM-808).
// Uses generated builder from docs/process/command_specs/object/evoman_command.yaml; subcommands add RunE and query flags.
func NewEvomanCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectEvomanCommandBuilder()
	cmd.AddCommand(NewEvomanShowCmd())
	cmd.AddCommand(NewEvomanListCmd())
	return cmd
}
