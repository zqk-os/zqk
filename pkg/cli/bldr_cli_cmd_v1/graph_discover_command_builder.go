package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewGraphDiscoverCommandBuilder creates a new graph_discover command
func NewGraphDiscoverCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for discover")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
