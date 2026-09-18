package spec

import (
	"fmt"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

// NewSpecCmd creates the "spec" command group for programmatic spec management (CRIT-9036, BLI-152).
func NewSpecCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Spec operations (list and manage object specifications)",
		"Programmatic management of object specifications (object_spec).",
		"",
		"This command group provides operations for spec management:",
		"  list   - List available object specifications (from storage or bundled)",
		"  get    - Get specification details for an object kind",
		"  fields - Inspect fields and validation rules for an object kind",
	).
		AddExample("List specs", "%s spec list").
		AddExample("List specs as JSON", "%s spec list --format json").
		AddExample("Get spec details", "%s spec get backlog_item").
		AddExample("Inspect spec fields", "%s spec fields backlog_item")

	specCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSpecCommandBuilder(), &cobra.Command{
		Use:   "spec",
		Short: "Spec operations (list and manage object specifications)",
		Long:  fmt.Sprintf("Programmatic management of object specifications.\n\nExamples:\n  %s spec list\n  %s spec get backlog_item\n  %s spec fields backlog_item", paths.CLICommandName, paths.CLICommandName, paths.CLICommandName),
	})

	helpBuilder.ApplyToCommand(specCmd)

	specCmd.AddCommand(NewSpecListCmd())
	specCmd.AddCommand(NewSpecGetCmd())
	specCmd.AddCommand(NewSpecFieldsCmd())

	return specCmd
}
