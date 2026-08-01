package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerBundleProgressCommandBuilder creates a new scheduler_bundle_progress command
func NewSchedulerBundleProgressCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("bundle-progress")
	builder.WithShort("Render live test bundle execution progress matrix")
	help := clipkg.DynamicHelpBuilder("Render live test bundle execution progress matrix")
	help.AddExample("View live bundle execution progress", "%s scheduler bundle-progress")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
