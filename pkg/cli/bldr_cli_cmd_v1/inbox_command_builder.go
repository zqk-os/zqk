package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewInboxCommandBuilder creates a new inbox command
func NewInboxCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("inbox")
	builder.WithShort("Manage the Autonomy Inbox for agent proposals and instructions")
	help := clipkg.DynamicHelpBuilder("Manage the Autonomy Inbox for agent proposals and instructions")
	help.WithDescriptionLines("The inbox command group provides access to the Autonomy Inbox Protocol. Agents and workflows submit proposals (agent_instruction objects) into the inbox. Human operators review, approve, or reject these proposals before execution.")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
