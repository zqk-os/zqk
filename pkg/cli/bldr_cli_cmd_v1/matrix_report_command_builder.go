package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewMatrixReportCommandBuilder creates a new matrix_report command
func NewMatrixReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("report")
	builder.WithShort("Summarize a traceability matrix (gates, completion counts)")
	help := clipkg.DynamicHelpBuilder("Summarize a traceability matrix (gates, completion counts)")
	help.WithDescriptionLines("Loads docs/quality/matrix_registry.yaml (or --registry), resolves the named matrix to a CSV + profile YAML,")
	help.WithDescriptionLines("and prints completion statistics (per-gate counts, fully-done rows). Aligns with docs/architecture/MATRIX_CLI_STRATEGY.md.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("For test-bundle inventories, outcomes such as pass/fail appear under \"other\" unless listed in the profile done_values.")
	help.AddExample("Report default codebase vetting matrix", "%s matrix report")
	help.AddExample("Report test-bundle matrix", "%s matrix report --name test_bundle")
	help.AddExample("Go files only", "%s matrix report --go-only")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("name", "", "codebase_vetting", "Matrix alias from matrix_registry.yaml (default: codebase_vetting)")
	builder.AddStringFlag("registry", "", "", "Path to matrix_registry.yaml (repo-relative or absolute); default docs/quality/matrix_registry.yaml")
	builder.AddStringFlag("matrix", "", "", "Override CSV path (bypasses registry name resolution)")
	builder.AddStringFlag("profile", "", "", "Override profile YAML path when using --matrix")
	builder.AddBoolFlag("go-only", "", false, "Only count rows whose file_path ends with .go (codebase vetting)")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
