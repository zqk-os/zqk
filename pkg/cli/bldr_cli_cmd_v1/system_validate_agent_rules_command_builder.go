package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemValidateAgentRulesCommandBuilder creates a new system_validate_agent_rules command
func NewSystemValidateAgentRulesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate-agent-rules")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
