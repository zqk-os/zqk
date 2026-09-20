package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAggregateAuditCommandBuilder creates a new system_aggregate_audit command
func NewSystemAggregateAuditCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for aggregate-audit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
