package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemQuarantineReportCommandBuilder creates a new system_quarantine_report command
func NewSystemQuarantineReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for quarantine-report")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
