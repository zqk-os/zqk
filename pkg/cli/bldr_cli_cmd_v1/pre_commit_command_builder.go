package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitCommandBuilder creates a new pre_commit command
func NewPreCommitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("pre-commit")
	builder.WithShort("Run pre-commit repository validation hooks and verification suites")
	help := clipkg.DynamicHelpBuilder("Run pre-commit repository validation hooks and verification suites")
	help.WithDescriptionLines("Executes pre-commit verification gates, linting checks, and AST integrity policies before repository commits.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
