package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectBulkDeleteCommandBuilder creates a new object_bulk_delete command
func NewObjectBulkDeleteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("delete", "delete")
	builder.WithDryRunFlag()
	builder.WithCascadeFlag()
	builder.WithUnlinkReferencesFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddStringFlag("ids", "", "", "Comma-separated list of object IDs")
	builder.AddStringFlag("file", "", "", "Path to YAML file containing array of IDs")
	builder.AddStringArrayFlag("fields", "", "Top-level keys for each successful delete entry in formatted output (when stored).")
	builder.WithShort("Delete multiple objects by ID")
	help := clipkg.DynamicHelpBuilder("Delete multiple objects by ID")
	help.WithDescriptionLines("Delete multiple objects by their IDs.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("You can provide IDs either:")
	help.WithDescriptionLines("  - Via --ids flag (comma-separated): --ids ITEM-001,ITEM-002,ITEM-003")
	help.WithDescriptionLines("  - Via --file flag (YAML array of IDs): --file ids.yaml")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("By default, deletion will fail if any object has dependents.")
	help.WithDescriptionLines("Use --unlink-references to remove each ID from dependents' reference fields before deleting (matches single-object delete).")
	help.WithDescriptionLines("Use --cascade to delete objects and all their dependents recursively.")
	help.WithDescriptionLines("Do not combine --unlink-references with --cascade.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Optional --fields restricts each successful delete result row to those top-level keys when present.")
	help.AddExample("Delete multiple objects", "%s object bulk delete --ids ITEM-001,ITEM-002,ITEM-003")
	help.AddExample("Unlink references from dependents then delete", "%s object bulk delete --ids CRIT-001,CRIT-002 --unlink-references")
	help.AddExample("Delete with cascade", "%s object bulk delete --ids ITEM-001,ITEM-002 --cascade")
	help.AddExample("Delete from file", "%s object bulk delete --file ids.yaml --cascade")
	help.AddExample("Dry-run to see what would be deleted", "%s object bulk delete --ids ITEM-001,ITEM-002 --cascade --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
