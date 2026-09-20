package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRemindersCommandBuilder creates a new system_reminders command
func NewSystemRemindersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for reminders")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
