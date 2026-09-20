package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemAgentLockdownCommandBuilder creates a new system_agent_lockdown command
func NewSystemAgentLockdownCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for agent-lockdown")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
