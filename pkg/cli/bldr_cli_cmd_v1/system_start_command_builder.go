package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStartCommandBuilder creates a new system_start command
func NewSystemStartCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for start")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
