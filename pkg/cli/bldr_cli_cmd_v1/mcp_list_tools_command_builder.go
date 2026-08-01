package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpListToolsCommandBuilder creates a new mcp_list_tools command
func NewMcpListToolsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list-tools")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
