package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewStartCommandBuilder creates a new start command
func NewStartCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("start")
	builder.WithShort("Start the scheduler daemon")
	help := clipkg.DynamicHelpBuilder("Start the scheduler daemon")
	help.WithDescriptionLines("Start the scheduler daemon to execute scheduled jobs.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("The daemon will:")
	help.WithDescriptionLines("  - Load all enabled scheduler_job objects")
	help.WithDescriptionLines("  - Schedule timer-based jobs according to their cron expressions")
	help.WithDescriptionLines("  - Listen for event and lifecycle triggers")
	help.WithDescriptionLines("  - Execute jobs with timeout and retry logic")
	help.WithDescriptionLines("  - Create audit events for job execution")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Default: runs in the background (detached process). Use --foreground to attach and stream logs.")
	help.WithDescriptionLines("If the daemon exits (crash or stop), run zqk scheduler start again to bring it back.")
	help.AddExample("Start scheduler daemon", "%s scheduler start")
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
