package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewCompletionCommandBuilder creates a new completion command
func NewCompletionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("completion")
	builder.WithShort("Generate shell completion scripts for supported shells")
	help := clipkg.DynamicHelpBuilder("Generate shell completion scripts for supported shells")
	help.WithDescriptionLines(
		"Generate shell auto-completion scripts for zqk CLI commands across",
		"supported shells including bash, zsh, fish, and powershell.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
