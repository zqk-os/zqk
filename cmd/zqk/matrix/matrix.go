package matrix

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	bldr_cli_cmd_v1 "github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewMatrixCmd is the top-level matrix command group (traceability / vetting CSVs).
// See docs/architecture/MATRIX_CLI_STRATEGY.md
func NewMatrixCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Traceability matrices (vetting CSVs, registry, reports)",
		"Work with named matrices defined in "+filepath.Join(paths.DocsQualityDir, "matrix_registry.yaml")+": reports, row updates (gate columns),",
		"and CVS linkage via CSV columns. Prefer these commands over one-off scripts for discoverability.",
		"",
		"Registry keys include: codebase_vetting, test_bundle.",
	).
		AddExample("Summarize default codebase vetting matrix", "%s matrix report").
		AddExample("Test-bundle matrix", "%s matrix report --name test_bundle").
		AddExample("Update one vetting row", "%s matrix update --file-path pkg/x.go --set fully_vetted=yes").
		AddExample("List pending vetting rows", "%s matrix get --filter fully_vetted=pending").
		AddExample("Bulk-set gate for all pending rows", "%s matrix update --filter fully_vetted=pending --set fully_vetted=yes").
		AddExample("List registry matrix names", "%s matrix list").
		AddExample("Check registry CSVs and profiles", "%s matrix validate")

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewMatrixCommandBuilder(), &cobra.Command{
		Use: "matrix",
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.AddCommand(NewMatrixReportCmd())
	cmd.AddCommand(NewMatrixListCmd())
	cmd.AddCommand(NewMatrixGetCmd())
	cmd.AddCommand(NewMatrixUpdateCmd())
	return cmd
}

// NewMatrixReportCmd wires the generated spec builder to RunE.
func NewMatrixReportCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixReportCommandBuilder()
	c.RunE = runMatrixReport
	return c
}

// NewMatrixListCmd wires the generated list spec builder to RunE.
func NewMatrixListCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixListCommandBuilder()
	c.RunE = runMatrixList
	return c
}

// NewMatrixGetCmd wires the generated get spec builder to RunE.
func NewMatrixGetCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixGetCommandBuilder()
	cli.BindAsyncProgress(c, runMatrixGet)
	return c
}

// NewMatrixUpdateCmd wires the generated update spec builder to RunE.
func NewMatrixUpdateCmd() *cobra.Command {
	c := bldr_cli_cmd_v1.NewMatrixUpdateCommandBuilder()
	cli.BindAsyncProgress(c, runMatrixUpdate)
	return c
}
