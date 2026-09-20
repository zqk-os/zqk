package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInboxListCommandBuilder creates a new inbox_list command
func NewInboxListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("list")
	builder.WithShort("List items in the Autonomy Inbox")
	help := clipkg.DynamicHelpBuilder("List items in the Autonomy Inbox")
	help.WithDescriptionLines("List agent_instruction proposals currently pending in the inbox. Supports filtering by status and persona.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
