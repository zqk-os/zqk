package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAuditMilestonesCommandBuilder creates a new system_audit_milestones command
func NewSystemAuditMilestonesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for audit-milestones")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
