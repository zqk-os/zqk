package utility

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewUtilityCmd creates a new utility command group
func NewUtilityCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Utility operations (version, migration, and helpers)",
		"Utility operations for system information and data management.",
		"",
		"This command group provides utility operations:",
		"  Common operations: version, migrate, validate-yaml, fix-hashes",
		"  Specialized operations: (future: export, import, etc.)",
	).
		AddExample("Show version", "%s utility version").
		AddExample("Migrate data", "%s utility migrate --from file --to graph").
		AddExample("Validate YAML", "%s utility validate-yaml file.yaml").
		AddExample("Fix hashes", "%s utility fix-hashes --recursive "+paths.ProcessDir+"/")

	utilityCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewUtilityCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use: "utility",
	})

	helpBuilder.ApplyToCommand(utilityCmd)

	// Common operations
	utilityCmd.AddCommand(NewVersionCmd())
	utilityCmd.AddCommand(NewMigrateCmd())
	utilityCmd.AddCommand(NewValidateYAMLCmd())
	utilityCmd.AddCommand(NewFixHashesCmd())
	utilityCmd.AddCommand(NewFixRegistrationCmd())
	utilityCmd.AddCommand(NewScenarioBuilderCmd())

	// Specialized operations (future)
	// utilityCmd.AddCommand(NewExportCmd())
	// utilityCmd.AddCommand(NewImportCmd())

	return utilityCmd
}
