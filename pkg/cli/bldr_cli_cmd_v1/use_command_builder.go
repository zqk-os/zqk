package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewUseCommandBuilder creates a new use command
func NewUseCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("use")
	builder.WithShort("Switch active context or environment configuration")
	help := clipkg.DynamicHelpBuilder("Switch active context or environment configuration")
	help.WithDescriptionLines(
		"Select and activate an operational context, target cluster, or environment",
		"configuration for subsequent CLI invocations.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
