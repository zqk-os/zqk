package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewDocmanRegisterCommandBuilder creates a new docman_register command
func NewDocmanRegisterCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for register")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
