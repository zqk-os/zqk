package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpCommandBuilder creates a new mcp command
func NewMcpCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for mcp")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
