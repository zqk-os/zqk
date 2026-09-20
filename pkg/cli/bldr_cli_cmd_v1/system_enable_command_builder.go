package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemEnableCommandBuilder creates a new system_enable command
func NewSystemEnableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for enable")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
