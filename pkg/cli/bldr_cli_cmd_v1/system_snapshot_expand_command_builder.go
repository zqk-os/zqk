package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSnapshotExpandCommandBuilder creates a new system_snapshot_expand command
func NewSystemSnapshotExpandCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for snapshot-expand")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
