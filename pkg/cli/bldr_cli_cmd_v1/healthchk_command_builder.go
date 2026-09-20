package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkCommandBuilder creates a new healthchk command
func NewHealthchkCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for healthchk")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
