package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemInterruptInboxCommandBuilder creates a new system_interrupt_inbox command
func NewSystemInterruptInboxCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for interrupt-inbox")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
