package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemDashboardCommandBuilder creates a new system_dashboard command
func NewSystemDashboardCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("dashboard")
	builder.WithShort("dashboard command")
	help := clipkg.DynamicHelpBuilder("dashboard command")
	help.WithDescriptionLines("dashboard command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
