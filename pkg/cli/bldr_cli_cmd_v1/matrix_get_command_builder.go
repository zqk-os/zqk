package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMatrixGetCommandBuilder creates a new matrix_get command
func NewMatrixGetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("get")
	builder.WithShort("List matrix CSV rows matching filters")
	help := clipkg.DynamicHelpBuilder("List matrix CSV rows matching filters")
	help.WithDescriptionLines("Resolves CSV + profile via docs/quality/matrix_registry.yaml (same as matrix report), reads the")
	help.WithDescriptionLines("inventory, and returns rows matching optional --filter column=value (AND), --glob on file_path")
	help.WithDescriptionLines("(filepath.Match), --go-only, and --cvs-id against the registry session_ref_column. Use --field")
	help.WithDescriptionLines("(repeatable) to narrow output to specific CSV columns (order preserved). Does not collide with")
	help.WithDescriptionLines("global --columns (table column widths).")
	help.AddExample("All rows (default matrix)", "%s matrix get")
	help.AddExample("Filter by gate column", "%s matrix get --filter fully_vetted=pending")
	help.AddExample("Go files matching a path glob", "%s matrix get --go-only --glob 'pkg/*.go'")
	help.AddExample("Only file_path and gate column in output", "%s matrix get --field file_path --field fully_vetted")
	help.AddExample("Pipe-friendly CSV (header + data rows)", "%s matrix get --format csv --filter fully_vetted=pending")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("name", "", "", "Matrix alias from matrix_registry.yaml (default: resolved from registry default)")
	builder.AddStringFlag("registry", "", "", "Path to matrix_registry.yaml (repo-relative or absolute)")
	builder.AddStringFlag("matrix", "", "", "Override CSV path (requires --profile)")
	builder.AddStringFlag("profile", "", "", "Override profile YAML when using --matrix")
	builder.AddStringArrayFlag("filter", "F", "column=value (repeatable); all must match (AND)")
	builder.AddStringFlag("glob", "", "", "Glob pattern for file_path column (filepath.Match; e.g. pkg/*.go or */foo.go)")
	builder.AddBoolFlag("go-only", "", false, "Only rows whose file_path ends with .go")
	builder.AddStringFlag("cvs-id", "", "", "Only rows whose session ref column equals this id (uses registry session_ref_column)")
	builder.AddIntFlag("limit", "", 0, "Max matching rows (0 = no limit)")
	builder.AddStringArrayFlag("field", "C", "CSV column name to include in output (repeatable); order preserved; default is all columns")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
