package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMcpInstallCommandBuilder creates a new mcp_install command
func NewMcpInstallCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for install")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
