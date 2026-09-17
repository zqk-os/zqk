package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCleanupQuarantineCommandBuilder creates a new system_cleanup_quarantine command
func NewSystemCleanupQuarantineCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for cleanup-quarantine")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
