package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewObjectBulkCreateCommandBuilder creates a new object_bulk_create command
func NewObjectBulkCreateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("create", "create <kind>")
	builder.WithDryRunFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddStringFlag("file", "", "", "Path to YAML file containing array of objects (required)")
	builder.AddBoolFlag("relaxed", "", false, "Relax integrity constraints during bulk creation (allow references to objects that will be created later in batch operations). Note: Auto-detected for bulk operations, this flag is optional.")
	builder.AddBoolFlag("force", "", false, "Force mode: update existing objects instead of failing if they already exist")
	builder.AddStringArrayFlag("fields", "", "Top-level keys per successful object in output (hybrid projection; post-create).")
	builder.WithShort("Create multiple objects from a file")
	help := clipkg.DynamicHelpBuilder("Create multiple objects from a file")
	help.WithDescriptionLines("Create multiple objects of the specified kind from a YAML file.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The file should contain a YAML array of objects:")
	help.WithDescriptionLines("  - kind: backlog_item")
	help.WithDescriptionLines("    title: \"Item 1\"")
	help.WithDescriptionLines("    ...")
	help.WithDescriptionLines("  - kind: backlog_item")
	help.WithDescriptionLines("    title: \"Item 2\"")
	help.WithDescriptionLines("    ...")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Optional --fields restricts each successful created object in JSON/YAML output to those top-level keys")
	help.WithDescriptionLines("(same hybrid projection as object list / get).")
	help.AddExample("Create multiple backlog items from file", "%s object bulk create backlog_item --file items.yaml")
	help.AddExample("Dry-run to see what would be created", "%s object bulk create backlog_item --file items.yaml --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
