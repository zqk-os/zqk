package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewVersionCommandBuilder creates a new version command
func NewVersionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("version")
	builder.WithShort("Display ZQK binary version, build commit, and architecture metadata")
	help := clipkg.DynamicHelpBuilder("Display ZQK binary version, build commit, and architecture metadata")
	help.WithDescriptionLines("Outputs semantic version, git commit SHA, build timestamp, Go toolchain version, and runtime platform architecture.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
