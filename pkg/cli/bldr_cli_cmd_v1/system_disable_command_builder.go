package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemDisableCommandBuilder creates a new system_disable command
func NewSystemDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for disable")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
