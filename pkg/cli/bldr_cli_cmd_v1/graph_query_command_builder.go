package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewGraphQueryCommandBuilder creates a new graph_query command
func NewGraphQueryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for query")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
