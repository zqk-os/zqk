package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewSkipWindowCommandBuilder creates a new skip_window command
func NewSkipWindowCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("skip-window")
	builder.WithShort("Set or clear scheduler skip window (bulk skip matching jobs until a time)")
	help := clipkg.DynamicHelpBuilder("Set or clear scheduler skip window (bulk skip matching jobs until a time)")
	help.WithDescriptionLines("Manage `.zqk/scheduler/skip_window.yaml`. When active, the scheduler policy engine skips")
	help.WithDescriptionLines("matching job executions until `until` (handlers do not run). Use during instability or when")
	help.WithDescriptionLines("you want timer-driven jobs to stay quiet until the workspace is healthy again.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Subcommands: set, clear, status.")
	help.AddExample("Skip convergence_session_tick timer jobs for 2 hours", "%s scheduler skip-window set --for 2h --job-type convergence_session_tick --trigger-type timer")
	help.AddExample("Skip all jobs for 30 minutes (use with care)", "%s scheduler skip-window set --for 30m --match-all")
	help.AddExample("Show active window", "%s scheduler skip-window status")
	help.AddExample("Remove skip window", "%s scheduler skip-window clear")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	cli.RequireStorage(cmd, false)
	cli.RequireSession(cmd, false)
	cli.RequireSchedulerCheck(cmd, false)
	return cmd
}
