package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentValidateCommandBuilder creates a new agent_validate command
func NewAgentValidateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
