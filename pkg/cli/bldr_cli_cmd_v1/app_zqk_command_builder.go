package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAppZqkCommandBuilder creates a new app_zqk command
func NewAppZqkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for zqk")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
