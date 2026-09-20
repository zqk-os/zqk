package organizational

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewOrganizationalCmd creates the organizational command group
func NewOrganizationalCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Organizational structure and change impact analysis",
		"Organizational operations for managing organizational structure and analyzing impact of changes.",
		"",
		"The organizational command group provides tools for:",
		"- Analyzing impact of organizational changes on kernel objects",
		"- Detecting organizational changes",
		"- Managing organizational structure relationships",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewOrganizationalCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "organizational",
	})

	helpBuilder.ApplyToCommand(cmd)

	// Add subcommands
	cmd.AddCommand(NewAnalyzeImpactCmd())
	cmd.AddCommand(NewPropagateCmd())
	cmd.AddCommand(NewRecordChangeCmd())
	cmd.AddCommand(NewSyncCmd())

	return cmd
}
