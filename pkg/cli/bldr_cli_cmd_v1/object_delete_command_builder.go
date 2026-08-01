package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewObjectDeleteCommandBuilder creates a new object_delete command
func NewObjectDeleteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("delete", "delete <id>")
	builder.WithDryRunFlag()
	builder.WithCascadeFlag()
	builder.WithUnlinkReferencesFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.WithShort("Delete an object by ID")
	help := clipkg.DynamicHelpBuilder("Delete an object by ID")
	help.WithDescriptionLines("Delete an object by its ID.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("By default, deletion will fail if the object has dependents (objects that reference it).")
	help.WithDescriptionLines("Use --unlink-references to remove this ID from dependents' reference fields (CASCADE_NULLIFY-style), then delete.")
	help.WithDescriptionLines("Use --cascade to delete the object and all its dependents recursively.")
	help.WithDescriptionLines("Do not combine --unlink-references with --cascade.")
	help.AddExample("Delete an object (fails if it has dependents)", "%s delete ITEM-626")
	help.AddExample("Drop references from dependents (nullify refs), then delete", "%s delete CRIT-002 --unlink-references")
	help.AddExample("Delete with cascade (deletes object and all dependents)", "%s delete ITEM-626 --cascade")
	help.AddExample("Dry-run to see what would be deleted", "%s delete ITEM-626 --cascade --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
