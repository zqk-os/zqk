package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSchedulerSkipWindowClearCommandBuilder creates a new scheduler_skip_window_clear command
func NewSchedulerSkipWindowClearCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("clear")
	builder.WithShort("Remove skip_window.yaml")
	help := clipkg.DynamicHelpBuilder("Remove skip_window.yaml")
	help.WithDescriptionLines("Deletes `.zqk/scheduler/skip_window.yaml` so policy evaluation no longer applies")
	help.WithDescriptionLines("bulk skip-window rules.")
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
