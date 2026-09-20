package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewVersionCommandBuilder creates a new version command
func NewVersionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("version")
	builder.WithShort("version command")
	help := clipkg.DynamicHelpBuilder("version command")
	help.WithDescriptionLines("version command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
