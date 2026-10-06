package object

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewPPlanCmd creates a new priority plan command group
func NewPPlanCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Priority plan operations (current)",
		"Priority plan operations for viewing active priority plans.",
		"",
		"This command group provides operations for working with priority plans:",
		"  - current: View backlog items for the current priority plan",
	).
		AddExample("View backlog items for current priority plan", "%s pplan current")

	pplanCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectPplanCommandBuilder(), &cobra.Command{
		Use:     "pplan",
		Aliases: []string{"plan"},
	})

	helpBuilder.ApplyToCommand(pplanCmd)

	pplanCmd.AddCommand(NewPPlanCurrentCmd())

	return pplanCmd
}
