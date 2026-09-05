package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemValidateAgentRulesCommandBuilder creates a new system_validate_agent_rules command
func NewSystemValidateAgentRulesCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate-agent-rules")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
