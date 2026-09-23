package state

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewStateCmd creates the 'zqk state' command group for inspecting the live kernel state and journals.
func NewStateCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"State — inspect live knowledge kernel state graph and audit journals",
		"Provides human-readable inspection of the live kernel state graph, active agent leases, and recent mutation journals.",
		"",
		"Use 'zqk state tree' to visualize the organizational and backlog execution hierarchy.",
	).
		AddExample("Render live execution state tree", "%s state tree").
		AddExample("Render state tree as JSON", "%s state tree --format json").
		AddExample("List recent journal mutations", "%s state journal --limit 10")

	cmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect live knowledge kernel state graph and audit journals",
		Long:  "Provides human-readable inspection of the live kernel state graph, active agent leases, and recent mutation journals.",
	}
	helpBuilder.ApplyToCommand(cmd)

	cmd.AddCommand(newTreeCmd())
	cmd.AddCommand(newJournalCmd())

	return cmd
}
