package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAuditReportCommandBuilder creates a new system_audit_report command
func NewSystemAuditReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for audit-report")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
