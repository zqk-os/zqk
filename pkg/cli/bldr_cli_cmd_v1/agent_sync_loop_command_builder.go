package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentSyncLoopCommandBuilder creates a new agent_sync_loop command
func NewAgentSyncLoopCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
