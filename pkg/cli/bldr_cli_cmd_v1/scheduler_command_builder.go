package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerCommandBuilder creates a new scheduler command
func NewSchedulerCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("scheduler")
	builder.WithShort("scheduler command")
	help := clipkg.DynamicHelpBuilder("scheduler command")
	help.WithDescriptionLines("scheduler command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
