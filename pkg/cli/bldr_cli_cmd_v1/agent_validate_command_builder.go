package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentValidateCommandBuilder creates a new agent_validate command
func NewAgentValidateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for validate")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
