package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewStatusCommandBuilder creates a new status command
func NewStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("status")
	builder.WithShort("Show scheduler daemon status")
	help := clipkg.DynamicHelpBuilder("Show scheduler daemon status")
	help.WithDescriptionLines("Show the current status of the scheduler daemon and running jobs.")
	help.AddExample("Show scheduler status", "%s scheduler status")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
