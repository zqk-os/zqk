package object

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewBulkCmd creates a new bulk command with subcommands
func NewBulkCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Bulk operations for multiple objects",
		"Perform bulk operations on multiple objects at once.",
		"",
		"Bulk operations are atomic - either all operations succeed or all fail (transaction-based).",
		"For operations that can partially succeed (like bulk-get), errors are reported per-object.",
		"",
		"Available subcommands:",
		"  - create: Create multiple objects from a file",
		"  - update: Update multiple objects from a file",
		"  - get: Get multiple objects by ID",
		"  - delete: Delete multiple objects by ID",
	).
		AddExample("Create multiple objects", "%s object bulk create backlog_item --file items.yaml").
		AddExample("Update multiple objects", "%s object bulk update --file updates.yaml").
		AddExample("Get multiple objects", "%s object bulk get --ids BLI-001,BLI-002,BLI-003")

	bulkCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectBulkCommandBuilder(), &cobra.Command{
		Use:        "bulk",
		Deprecated: "Use standard 'object <op>' commands natively with multiple arguments or --file flags instead",
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(bulkCmd)

	bulkCmd.AddCommand(NewBulkCreateCmd())
	bulkCmd.AddCommand(NewBulkUpdateCmd())
	bulkCmd.AddCommand(NewBulkGetCmd())
	bulkCmd.AddCommand(NewBulkDeleteCmd())

	return bulkCmd
}
