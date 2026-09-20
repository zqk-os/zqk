package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemEmitCommandBuilder creates a new system_emit command
func NewSystemEmitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for emit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
