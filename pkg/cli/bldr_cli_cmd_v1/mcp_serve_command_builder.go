package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpServeCommandBuilder creates a new mcp_serve command
func NewMcpServeCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for serve")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
