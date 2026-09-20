package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemAckCommandBuilder creates a new system_ack command
func NewSystemAckCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for ack")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
