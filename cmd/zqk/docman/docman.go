package docman

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

const emptyValue = ""

// NewDocmanCmd creates a new docman command group
func NewDocmanCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Documentation management operations",
		"Documentation management operations for discovering and registering documentation files.",
		"",
		"This command group provides operations for managing documentation:",
		"  - register: Discover and register markdown files as doc_entry objects",
		"  - verify: Cryptographically verify doc_entry objects against on-disk files and detect drift",
	).
		AddExample("Register all documentation files", "%s docman register").
		AddExample("Dry run to see what would be registered", "%s docman register --dry-run").
		AddExample("Verify shipped documentation integrity", "%s docman verify --shipped-only")

	docmanCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDocmanCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "docman",
	})

	helpBuilder.ApplyToCommand(docmanCmd)

	docmanCmd.AddCommand(NewRegisterCmd())
	docmanCmd.AddCommand(NewVerifyCmd())

	return docmanCmd
}
