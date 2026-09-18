package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemMaintenanceRequestCycleCommandBuilder creates a new system_maintenance_request_cycle command
func NewSystemMaintenanceRequestCycleCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("maintenance-request-cycle")
	builder.WithShort("Request one maintenance cycle via the maintenance WAL")
	help := clipkg.DynamicHelpBuilder("Request one maintenance cycle via the maintenance WAL")
	help.WithDescriptionLines("Appends a cycle_requested event to the maintenance WAL. If the scheduler daemon")
	help.WithDescriptionLines("is running, its maintenance runner will process it in order (audit aggregation")
	help.WithDescriptionLines("then retention). Safe to run when daemon is stopped; the event is durable.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Use this for on-demand catch-up or after increasing maintenance frequency.")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddBoolFlag("allow-degraded", "", false, "Allow running when scheduler daemon is not running (results may be partial/stale)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
