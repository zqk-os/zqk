package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAgentLockdownCommandBuilder creates a new system_agent_lockdown command
func NewSystemAgentLockdownCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for agent-lockdown")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
