package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitCommandBuilder creates a new pre_commit command
func NewPreCommitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("pre-commit")
	builder.WithShort("pre-commit command")
	help := clipkg.DynamicHelpBuilder("pre-commit command")
	help.WithDescriptionLines("pre-commit command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
