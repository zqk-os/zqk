package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAuditBufferCommandBuilder creates a new system_audit_buffer command
func NewSystemAuditBufferCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for audit-buffer")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
