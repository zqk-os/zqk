package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemPendingCommandBuilder creates a new system_pending command
func NewSystemPendingCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for pending")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
