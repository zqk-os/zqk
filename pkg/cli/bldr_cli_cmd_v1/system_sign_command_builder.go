package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSignCommandBuilder creates a new system_sign command
func NewSystemSignCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for sign")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
