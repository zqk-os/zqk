package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewGraphCommandBuilder creates a new graph command
func NewGraphCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for graph")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
