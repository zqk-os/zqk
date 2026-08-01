package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerSkipWindowStatusCommandBuilder creates a new scheduler_skip_window_status command
func NewSchedulerSkipWindowStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("status")
	builder.WithShort("Show active skip window if any")
	help := clipkg.DynamicHelpBuilder("Show active skip window if any")
	help.WithDescriptionLines("Prints the active skip window (until time and filters) when the file exists and")
	help.WithDescriptionLines("`until` is still in the future.")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
