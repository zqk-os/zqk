package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMatrixListCommandBuilder creates a new matrix_list command
func NewMatrixListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List named matrices from the matrix registry")
	help := clipkg.DynamicHelpBuilder("List named matrices from the matrix registry")
	help.WithDescriptionLines("Reads docs/quality/matrix_registry.yaml (or --registry) and prints each matrix alias with its")
	help.WithDescriptionLines("description and repo-relative csv/profile paths. Use to discover --name values for report, get, and update.")
	help.AddExample("Show all registry entries", "%s matrix list")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("registry", "", "", "Path to matrix_registry.yaml (repo-relative or absolute); default docs/quality/matrix_registry.yaml")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
