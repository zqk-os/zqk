package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemCommandBuilder creates a new system command
func NewSystemCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for system")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
