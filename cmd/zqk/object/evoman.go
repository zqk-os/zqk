package object

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewEvomanCmd creates the evolution management command group (BLI-808).
// Uses generated builder from .zqk/cli/specs/object/evoman_command.yaml; subcommands add RunE and query flags.
func NewEvomanCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectEvomanCommandBuilder()
	cmd.AddCommand(NewEvomanShowCmd())
	cmd.AddCommand(NewEvomanListCmd())
	return cmd
}
