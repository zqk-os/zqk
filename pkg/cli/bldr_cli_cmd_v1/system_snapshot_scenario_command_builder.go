package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSnapshotScenarioCommandBuilder creates a new system_snapshot_scenario command
func NewSystemSnapshotScenarioCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for snapshot-scenario")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
