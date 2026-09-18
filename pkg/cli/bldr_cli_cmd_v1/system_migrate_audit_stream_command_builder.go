package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemMigrateAuditStreamCommandBuilder creates a new system_migrate_audit_stream command
func NewSystemMigrateAuditStreamCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for migrate-audit-stream")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
