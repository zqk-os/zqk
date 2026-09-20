package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemCleanupQuarantineCommandBuilder creates a new system_cleanup_quarantine command
func NewSystemCleanupQuarantineCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for cleanup-quarantine")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
