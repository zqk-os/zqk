package domain

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewDomainCmd creates the domain command group
func NewDomainCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Domain ontology discovery and registration",
		"Domain operations for discovering and registering domain ontologies.",
		"",
		"The domain command group provides tools for:",
		"- Discovering registered domain ontologies (domain_registry objects)",
		"- Registering domain object types (future)",
		"- Validating cross-layer references (future)",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDomainCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "domain",
	})

	helpBuilder.ApplyToCommand(cmd)

	// Add subcommands
	cmd.AddCommand(NewDiscoverCmd())
	cmd.AddCommand(NewRegisterCmd())

	return cmd
}
