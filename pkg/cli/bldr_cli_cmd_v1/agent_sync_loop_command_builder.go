package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentSyncLoopCommandBuilder creates a new agent_sync_loop command
func NewAgentSyncLoopCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
