package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
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
