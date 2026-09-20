package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemConfigGetCommandBuilder creates a new system_config_get command
func NewSystemConfigGetCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for config-get")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
