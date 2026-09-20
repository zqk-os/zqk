package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemServiceCommandBuilder creates a new system_service command
func NewSystemServiceCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for service")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
