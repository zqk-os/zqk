package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewAgentChatResponderCommandBuilder creates a new agent_chat_responder command
func NewAgentChatResponderCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("chat-responder")
	builder.WithShort("Run the native Chat Responder subagent")
	help := clipkg.DynamicHelpBuilder("Run the native Chat Responder subagent")
	help.WithDescriptionLines("Reads the agent chat channel, determines if anything has changed since the last time it read, and responds natively within ZQK if there are new messages.")
	help.AddExample("Run chat responder", "%s agent chat-responder")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	cmd := builder.Build()
	return cmd
}
