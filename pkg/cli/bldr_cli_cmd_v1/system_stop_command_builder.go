package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemStopCommandBuilder creates a new system_stop command
func NewSystemStopCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for stop")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
