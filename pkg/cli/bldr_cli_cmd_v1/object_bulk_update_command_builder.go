package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewObjectBulkUpdateCommandBuilder creates a new object_bulk_update command
func NewObjectBulkUpdateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("update", "update <kind>")
	builder.WithDryRunFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.AddStringFlag("file", "", "", "YAML file: array of {id, updates: {field: value}}. Mutually exclusive with --filter/--set. Requires kind as first argument when set.")
	builder.AddStringArrayFlag("filter", "", "Filter objects to update (e.g., --filter status=planned). Can be repeated.")
	builder.AddStringArrayFlag("set", "s", "Set field values (e.g., --set status=in_progress). Can be repeated.")
	builder.AddStringFlag("kind", "", "", "Optional kind when not specified as the first argument or inferred from filter")
	builder.AddBoolFlag("relaxed", "", false, "Relax integrity constraints during bulk update (allow references to objects that will be created later in batch operations). Note: Auto-detected for bulk operations, this flag is optional.")
	builder.AddBoolFlag("force", "", false, "Force mode: skip 'not found' errors (treat missing objects as success)")
	builder.AddBoolFlag("keep-file", "", false, "Keep the source file after update (default: temp files like tmp-*.yaml are removed automatically)")
	builder.AddStringArrayFlag("fields", "", "Top-level keys per successful object in output (hybrid projection).")
	builder.WithShort("Bulk update objects (filter or file)")
	help := clipkg.DynamicHelpBuilder("Bulk update objects (filter or file)")
	help.WithDescriptionLines("Bulk update objects in two ways:")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("**Filter + set:** Match objects with --filter and apply --set field values (same kind).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("**File:** Pass --file with a YAML array of per-object updates:")
	help.WithDescriptionLines("  - id: BLI-001")
	help.WithDescriptionLines("    updates:")
	help.WithDescriptionLines("      title: \"Updated Title\"")
	help.WithDescriptionLines("      status: validated")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("When using --file, the object kind is required as the first argument (e.g.")
	help.WithDescriptionLines("`zqk object bulk update backlog_item --file updates.yaml`).")
	help.WithDescriptionLines("--file is mutually exclusive with --filter and --set.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Optional --fields restricts each updated object in the success payload to those top-level keys")
	help.WithDescriptionLines("(same hybrid projection as object get).")
	help.AddExample("Update by filter", "%s object bulk update backlog_item --filter status=planned --set status=in_progress")
	help.AddExample("Per-object updates from file", "%s object bulk update backlog_item --file updates.yaml")
	help.AddExample("Dry-run filter path", "%s object bulk update backlog_item --filter status=planned --set status=in_progress --dry-run")
	help.AddExample("Dry-run file path", "%s object bulk update backlog_item --file updates.yaml --dry-run")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	return builder.Build()
}
