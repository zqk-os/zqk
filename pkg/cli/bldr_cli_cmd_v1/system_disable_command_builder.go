package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemDisableCommandBuilder creates a new system_disable command
func NewSystemDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for disable")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
