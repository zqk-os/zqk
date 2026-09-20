package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSnapshotVerifyCommandBuilder creates a new system_snapshot_verify command
func NewSystemSnapshotVerifyCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for snapshot-verify")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
