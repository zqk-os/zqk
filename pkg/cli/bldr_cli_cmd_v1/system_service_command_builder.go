package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemServiceCommandBuilder creates a new system_service command
func NewSystemServiceCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for service")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
