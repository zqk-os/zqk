package bldr_cli_cmd_v1

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewMaterializeAgentChatChannelCommandBuilder creates a new materialize_agent_chat_channel command
func NewMaterializeAgentChatChannelCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("materialize-agent-chat-channel")
	builder.WithShort("Materialize agent_chat_channel lite file from a CAS agent_feed")
	help := clipkg.DynamicHelpBuilder("Materialize agent_chat_channel lite file from a CAS agent_feed")
	help.WithDescriptionLines("Reads an agent_feed (AGF-*) from CAS and writes `.zqk/config/agent_chat_channel.json`")
	help.WithDescriptionLines("(enabled, delivery_mode, feed_id, contract_schema_version, path overrides, probe filters).")
	help.WithDescriptionLines("Ensures the events JSONL parent directory exists.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("Prefer rematerializing from a live AGF after object updates instead of hand-editing the lite file.")
	help.AddExample("Materialize lite policy from a live agent_feed", "%s system materialize-agent-chat-channel --feed-id AGF-001")
	help.AddExample("Preview without writing", "%s system materialize-agent-chat-channel --feed-id AGF-001 --dry-run")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("feed-id", "", "", "agent_feed object id (AGF-*)")
	builder.AddBoolFlag("dry-run", "", false, "Print materialized lite JSON without writing")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down"})
	cmd := builder.Build()
	return cmd
}
