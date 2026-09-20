package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemRemindersCommandBuilder creates a new system_reminders command
func NewSystemRemindersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for reminders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
