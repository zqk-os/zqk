package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCacheAuditCommandBuilder creates a new system_cache_audit command
func NewSystemCacheAuditCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for cache-audit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
