package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewStopCommandBuilder creates a new stop command
func NewStopCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("stop")
	builder.WithShort("Stop the scheduler daemon")
	help := clipkg.DynamicHelpBuilder("Stop the scheduler daemon")
	help.WithDescriptionLines("Stop the running scheduler daemon.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("This gracefully stops the scheduler, allowing current jobs to complete")
	help.WithDescriptionLines("before shutting down.")
	help.AddExample("Stop scheduler daemon", "%s scheduler stop")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("wait", "", false, "")
	builder.AddStringFlag("max-wait", "", "3m", "")
	builder.AddBoolFlag("force", "", false, "")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
