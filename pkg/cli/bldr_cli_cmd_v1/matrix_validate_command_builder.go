package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMatrixValidateCommandBuilder creates a new matrix_validate command
func NewMatrixValidateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("validate")
	builder.WithShort("Validate matrix registry entries (CSV + profile)")
	help := clipkg.DynamicHelpBuilder("Validate matrix registry entries (CSV + profile)")
	help.WithDescriptionLines("Loads docs/quality/matrix_registry.yaml (or --registry), resolves csv and profile paths, checks files")
	help.WithDescriptionLines("exist, parses the profile, reads the CSV header and rows, and verifies session_ref_column when configured.")
	help.WithDescriptionLines("Exits with non-zero status if any matrix fails. Use --name to validate a single alias.")
	help.AddExample("Validate every registry matrix", "%s matrix validate")
	help.AddExample("Validate only test_bundle", "%s matrix validate --name test_bundle")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("name", "", "", "Matrix alias to validate only (default: all entries in the registry)")
	builder.AddStringFlag("registry", "", "", "Path to matrix_registry.yaml (repo-relative or absolute); default docs/quality/matrix_registry.yaml")
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
