package object

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewPPlanCmd creates a new priority plan command group
func NewPPlanCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Priority plan operations (current, next, prev, add, remove)",
		"Priority plan operations for navigating and managing priority plans.",
		"",
		"This command group provides operations for working with priority plans:",
		"  - current: View backlog items for the current priority plan",
		"  - next: Navigate to the next priority plan",
		"  - prev: Navigate to the previous priority plan",
		"  - add: Associate a backlog item with a priority plan (child-owned)",
		"  - remove: Clear priority plan association from a backlog item",
	).
		AddExample("View backlog items for current priority plan", "%s pplan current").
		AddExample("Add backlog item to priority plan", "%s pplan add PRI-123 BLI-456").
		AddExample("Alternative positional syntax", "%s pplan PRI-123 add BLI-456").
		AddExample("Remove backlog item from priority plan", "%s pplan remove PRI-123 BLI-456")

	pplanCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectPplanCommandBuilder(), &cobra.Command{
		Use: "pplan",
	})

	helpBuilder.ApplyToCommand(pplanCmd)

	pplanCmd.AddCommand(NewPPlanCurrentCmd())
	pplanCmd.AddCommand(NewPPlanNextCmd())
	pplanCmd.AddCommand(NewPPlanPrevCmd())
	pplanCmd.AddCommand(NewPPlanAddCmd())
	pplanCmd.AddCommand(NewPPlanRemoveCmd())

	// Support positional syntax fallback: zqk pplan <planID> add <bliID...> or zqk pplan <planID> remove <bliID...>
	pplanCmd.Args = cobra.ArbitraryArgs
	pplanCmd.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) >= 3 {
			if args[1] == "add" {
				return executePPlanAdd(cmd, args[0], args[2:]...)
			} else if args[1] == "remove" {
				return executePPlanRemove(cmd, args[0], args[2:]...)
			}
		}
		if len(args) >= 2 {
			if args[0] == "add" {
				planID, bliIDs := parsePlanAndBLIs(args[1:])
				return executePPlanAdd(cmd, planID, bliIDs...)
			} else if args[0] == "remove" {
				planID, bliIDs := parsePlanAndBLIs(args[1:])
				return executePPlanRemove(cmd, planID, bliIDs...)
			}
		}
		return cmd.Help()
	}

	return pplanCmd
}
