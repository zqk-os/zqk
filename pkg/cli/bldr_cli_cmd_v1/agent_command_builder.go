package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAgentCommandBuilder creates a new agent command
func NewAgentCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for agent")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
