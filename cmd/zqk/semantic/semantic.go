package semantic

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewSemanticCmd creates the semantic command group
func NewSemanticCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Semantic operations and maturity assessment",
		"Semantic operations for assessing and managing semantic maturity.",
		"",
		"The semantic command group provides tools for:",
		"- Assessing organization's semantic maturity level",
		"- Importing and managing ontologies",
		"- Working with semantic structures",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSemanticCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "semantic",
	})

	helpBuilder.ApplyToCommand(cmd)

	// Add subcommands
	cmd.AddCommand(
		NewAssessCmd(),
		NewRecommendCmd(),
		NewInferCmd(),
	)

	return cmd
}
