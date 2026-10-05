package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewConfigCommandBuilder creates a new config command
func NewConfigCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("config")
	builder.WithShort("Show or set scheduler config (jobs_paused)")
	help := clipkg.DynamicHelpBuilder("Show or set scheduler config (jobs_paused)")
	help.WithDescriptionLines("Show or update scheduler configuration (e.g. jobs_paused).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Without flags, shows the current config. Use --jobs-paused or --no-jobs-paused to set jobs_paused.")
	help.WithDescriptionLines("When jobs_paused is true, the scheduler (after restart) will not run timer or immediate jobs;")
	help.WithDescriptionLines("only manually triggered jobs run. Restart the scheduler for changes to take effect.")
	help.AddExample("Show current scheduler config", "%s scheduler config")
	help.AddExample("Pause automatic jobs (only manual trigger runs after restart)", "%s scheduler config --jobs-paused")
	help.AddExample("Resume normal scheduling", "%s scheduler config --no-jobs-paused")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("jobs-paused", "", false, "Set jobs_paused to true (no timer/immediate jobs; only manual trigger)")
	builder.AddBoolFlag("no-jobs-paused", "", false, "Set jobs_paused to false (normal scheduling)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
