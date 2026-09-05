package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMatrixUpdateCommandBuilder creates a new matrix_update command
func NewMatrixUpdateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("update")
	builder.WithShort("Update one or more rows in a traceability matrix CSV")
	help := clipkg.DynamicHelpBuilder("Update one or more rows in a traceability matrix CSV")
	help.WithDescriptionLines("Resolves CSV + profile via docs/quality/matrix_registry.yaml (same as matrix report), finds row(s)")
	help.WithDescriptionLines("by --file-path (file_path column), --bundle-label (bundle_label column), or --filter column=value")
	help.WithDescriptionLines("(repeatable, AND) for bulk updates, applies each --set column=value, validates gate columns against")
	help.WithDescriptionLines("profile completion.done_values, then replaces the CSV atomically. Use --dry-run to validate without writing.")
	help.AddExample("Set a vetting gate for one file", "%s matrix update --file-path pkg/foo/bar.go --set fully_vetted=yes")
	help.AddExample("Test-bundle row by label (non-gate column example)", "%s matrix update --name test_bundle --bundle-label my-bundle --set notes=rechecked")
	help.AddExample("Dry-run", "%s matrix update --file-path pkg/x.go --set fully_vetted=na --dry-run")
	help.AddExample("Bulk: set gate for all rows matching filters", "%s matrix update --filter fully_vetted=pending --set fully_vetted=yes")
	help.AddExample("Keep a .bak copy before writing", "%s matrix update --file-path pkg/x.go --set fully_vetted=yes --backup")
	help.AddExample("Append matrix update note to a CVS activity_log after write", "%s matrix update --file-path pkg/x.go --set fully_vetted=yes --append-cvs-activity --cvs-id CVS-abc123")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("name", "", "codebase_vetting", "Matrix alias from matrix_registry.yaml")
	builder.AddStringFlag("registry", "", "", "Path to matrix_registry.yaml (repo-relative or absolute)")
	builder.AddStringFlag("matrix", "", "", "Override CSV path (requires --profile)")
	builder.AddStringFlag("profile", "", "", "Override profile YAML when using --matrix")
	builder.AddStringFlag("file-path", "", "", "Match one row where file_path equals this value (normalized); mutually exclusive with --bundle-label and --filter")
	builder.AddStringFlag("bundle-label", "", "", "Match one row where bundle_label equals this value; mutually exclusive with --file-path and --filter")
	builder.AddStringArrayFlag("filter", "F", "column=value (repeatable, AND) to select rows for bulk update; mutually exclusive with --file-path and --bundle-label")
	builder.AddIntFlag("limit", "", 0, "With --filter, max rows to update in match order (0 = no limit)")
	builder.AddStringArrayFlag("set", "s", "column=value (repeatable). Gate columns must be allowed done_values in the profile.")
	builder.AddBoolFlag("dry-run", "", false, "Validate and print result without writing the CSV")
	builder.AddBoolFlag("backup", "", false, "Before replace, copy the current CSV to <csv>.bak (same directory)")
	builder.AddStringFlag("backup-to", "", "", "Before replace, copy the current CSV to this path (overrides --backup default)")
	builder.AddBoolFlag("append-cvs-activity", "", false, "After a successful CSV write, append one activity_log entry to each target convergence_session (use --cvs-id and/or registry session_ref_column on updated rows)")
	builder.AddStringArrayFlag("cvs-id", "", "Convergence session id(s) to append to (repeatable); when omitted, uses session_ref_column values from updated row(s)")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
