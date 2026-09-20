package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemResetTriggerQueueCommandBuilder creates a new system_reset_trigger_queue command
func NewSystemResetTriggerQueueCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for reset-trigger-queue")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
