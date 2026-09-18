package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemOrphanCleanupFallbackCommandBuilder creates a new system_orphan_cleanup_fallback command
func NewSystemOrphanCleanupFallbackCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for orphan-cleanup-fallback")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
