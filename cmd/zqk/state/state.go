package state

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewStateCmd creates the 'zqk state' command group for inspecting the live kernel state and journals.
func NewStateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewStateCommandBuilder()

	cmd.AddCommand(newTreeCmd())
	cmd.AddCommand(newJournalCmd())
	cmd.AddCommand(newStreamCmd())
	cmd.AddCommand(newTsdbCmd())

	return cmd
}
