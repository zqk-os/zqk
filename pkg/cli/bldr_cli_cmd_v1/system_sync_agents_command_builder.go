package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemSyncAgentsCommandBuilder creates a new system_sync_agents command
func NewSystemSyncAgentsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for sync-agents")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
