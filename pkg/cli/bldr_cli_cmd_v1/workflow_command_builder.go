package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowCommandBuilder creates a new workflow command
func NewWorkflowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("workflow")
	builder.WithShort("workflow command")
	help := clipkg.DynamicHelpBuilder("workflow command")
	help.WithDescriptionLines("workflow command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
