package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemMetricsFileLockCommandBuilder creates a new system_metrics_file_lock command
func NewSystemMetricsFileLockCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("file-lock")
	builder.WithShort("file-lock command")
	help := clipkg.DynamicHelpBuilder("file-lock command")
	help.WithDescriptionLines("file-lock command")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
