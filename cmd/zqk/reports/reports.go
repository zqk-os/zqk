package reports

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewReportsCmd creates a new reports command group
func NewReportsCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Generate AI metrics reports (PCS, EDD, D&B)",
		"Generate AI metrics reports including Project Confidence Score (PCS),",
		"Effort Distribution Discrepancy (EDD), and Dependencies & Blockers (D&B).",
		"",
		"These metrics are enhanced with Git commit data when available, providing",
		"predictive insights into project health, effort distribution, and potential",
		"blockers or dependencies.",
	).
		AddExample("Get Project Confidence Score", "%s reports pcs").
		AddExample("Get Effort Distribution Discrepancy", "%s reports edd").
		AddExample("Get Dependencies & Blockers", "%s reports blockers").
		AddExample("Get all metrics as JSON", "%s reports pcs --format json").
		AddExample("Quick lifecycle report (questions or milestones-overdue)", "%s reports quick --preset questions")

	reportsCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewReportsCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "reports",
	})

	helpBuilder.ApplyToCommand(reportsCmd)

	reportsCmd.AddCommand(NewPCSCmd())
	reportsCmd.AddCommand(NewEDDCmd())
	reportsCmd.AddCommand(NewBlockersCmd())
	reportsCmd.AddCommand(NewQuickCmd())

	return reportsCmd
}
