package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInternalBulkCommandBuilder creates a new internal_bulk command
func NewInternalBulkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("bulk")
	builder.WithShort("Bulk operations for multiple internal objects (admin only)")
	help := clipkg.DynamicHelpBuilder("Bulk operations for multiple internal objects (admin only)")
	help.WithDescriptionLines("Perform bulk operations on multiple internal or built-in objects at once.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Bulk operations are atomic - either all operations succeed or all fail (transaction-based).")
	help.WithDescriptionLines("For operations that can partially succeed (like bulk-get), errors are reported per-object.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Available subcommands:")
	help.WithDescriptionLines("  - create: Create multiple internal objects from a file")
	help.WithDescriptionLines("  - update: Update multiple internal objects from a file")
	help.WithDescriptionLines("  - get: Get multiple internal objects by ID")
	help.WithDescriptionLines("  - delete: Delete multiple internal objects by ID")
	help.AddExample("Create multiple internal objects", "%s internal bulk create kind_synonym --file synonyms.yaml")
	help.AddExample("Update multiple built-in objects", "%s internal bulk update --file updates.yaml")
	help.AddExample("Get multiple internal objects", "%s internal bulk get --ids KSYN-001,KSYN-002")
	help.AddExample("Delete multiple internal objects", "%s internal bulk delete --ids KSYN-001,KSYN-002 --cascade")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("ignore-scheduler-down", "", false, "Ignore scheduler down warning/lockdown")
	builder.AddBoolFlag("allow-degraded", "", false, "Allow running when scheduler daemon is not running (results may be partial/stale)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
