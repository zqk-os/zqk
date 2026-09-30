package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMetricsFileLockCommandBuilder creates a new system_metrics_file_lock command
func NewSystemMetricsFileLockCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("file-lock")
	builder.WithShort("Inspect file lock contention metrics and status")
	help := clipkg.DynamicHelpBuilder("Inspect file lock contention metrics and status")
	help.WithDescriptionLines(
		"Display metrics and telemetry for filesystem advisory locks,",
		"including contention durations, lock counts, and active holds.",
	)
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
