package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSnapshotExpandCommandBuilder creates a new system_snapshot_expand command
func NewSystemSnapshotExpandCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for snapshot-expand")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
