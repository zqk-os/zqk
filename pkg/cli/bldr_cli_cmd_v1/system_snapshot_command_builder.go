package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSnapshotCommandBuilder creates a new system_snapshot command
func NewSystemSnapshotCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for snapshot")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
