package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemObjectCountReportCommandBuilder creates a new system_object_count_report command
func NewSystemObjectCountReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for object-count-report")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
