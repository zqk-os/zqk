package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAuditStreamCommandBuilder creates a new system_audit_stream command
func NewSystemAuditStreamCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for audit-stream")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
