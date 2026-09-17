package object

import (
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewWstransCmd creates the workstream transition command group (BLI-807).
// Uses generated builder from .zqk/cli/specs/object/wstrans_command.yaml; subcommands add RunE and query flags.
func NewWstransCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectWstransCommandBuilder()
	cmd.AddCommand(NewWstransShowCmd())
	cmd.AddCommand(NewWstransListCmd())
	return cmd
}
