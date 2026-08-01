package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerIssuesCommandBuilder creates a new scheduler_issues command
func NewSchedulerIssuesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("issues")
	builder.WithShort("issues command")
	help := clipkg.DynamicHelpBuilder("issues command")
	help.WithDescriptionLines("issues command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
