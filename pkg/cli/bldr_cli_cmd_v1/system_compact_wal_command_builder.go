package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemCompactWalCommandBuilder creates a new system_compact_wal command
func NewSystemCompactWalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for compact-wal")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
