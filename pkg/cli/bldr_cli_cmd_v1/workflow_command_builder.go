package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowCommandBuilder creates a new workflow command
func NewWorkflowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("workflow")
	builder.WithShort("Manage and execute autonomous engineering workflows and lifecycle pipelines")
	help := clipkg.DynamicHelpBuilder("Manage and execute autonomous engineering workflows and lifecycle pipelines")
	help.WithDescriptionLines("Coordinates mission execution, priority plan delivery, Done-gate verification, and autonomous agent loops.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
