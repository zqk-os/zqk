package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemStopCommandBuilder creates a new system_stop command
func NewSystemStopCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for stop")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
