package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemQueryCommandBuilder creates a new system_query command
func NewSystemQueryCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("query")
	builder.WithShort("Generated spec for query")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
