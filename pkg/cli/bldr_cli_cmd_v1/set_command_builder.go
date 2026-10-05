package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewSetCommandBuilder creates a new set command
func NewSetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("set")
	builder.WithShort("Write skip_window.yaml (matching jobs skip until until)")
	help := clipkg.DynamicHelpBuilder("Write skip_window.yaml (matching jobs skip until until)")
	help.WithDescriptionLines("Writes `.zqk/scheduler/skip_window.yaml`. Requires `--for` or `--until`, and")
	help.WithDescriptionLines("`--match-all` and/or `--job-type` / `--trigger-type` / `--title-contains` filters.")
	help.AddExample("Two-hour window for convergence tick jobs", "%s scheduler skip-window set --for 2h --job-type convergence_session_tick --trigger-type timer")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("for", "", "", "Duration from now (e.g. 2h, 30m); mutually exclusive with --until")
	builder.AddStringFlag("until", "", "", "Absolute end time (RFC3339); mutually exclusive with --for")
	builder.AddBoolFlag("match-all", "", false, "Skip all jobs until until (use with care)")
	builder.AddStringArrayFlag("job-type", "", "Repeat for each job_type to match (e.g. convergence_session_tick)")
	builder.AddStringArrayFlag("trigger-type", "", "Repeat for each trigger_type to match (e.g. timer)")
	builder.AddStringFlag("title-contains", "", "", "Substring match on job title (case-insensitive)")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	cli.RequireStorage(cmd, false)
	cli.RequireSession(cmd, false)
	cli.RequireSchedulerCheck(cmd, false)
	return cmd
}
