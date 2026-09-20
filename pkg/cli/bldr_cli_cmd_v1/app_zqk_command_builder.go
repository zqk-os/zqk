package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewAppZqkCommandBuilder creates a new app_zqk command
func NewAppZqkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for zqk")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
