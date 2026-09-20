package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentCommandBuilder creates a new agent command
func NewAgentCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for agent")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
