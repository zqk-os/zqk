package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewSystemCompactMaintenanceWalCommandBuilder creates a new system_compact_maintenance_wal command
func NewSystemCompactMaintenanceWalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("compact-maintenance-wal")
	builder.WithShort("Compact maintenance WAL by removing applied entries")
	help := clipkg.DynamicHelpBuilder("Compact maintenance WAL by removing applied entries")
	help.WithDescriptionLines("Removes entries from .zqk/wal/maintenance.wal that have already been processed (seq <= checkpoint).")
	help.WithDescriptionLines("Run when the scheduler daemon is stopped so no process has the file open for append.")
	help.WithDescriptionLines("Routine compaction prevents maintenance.wal from growing indefinitely.")
	help.AddExample("Compact maintenance WAL (daemon stopped)", "%s system compact-maintenance-wal")
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
