package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemInitCommandBuilder creates a new system_init command
func NewSystemInitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for init")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
