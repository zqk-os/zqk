package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewWatchCommandBuilder creates a new watch command
func NewWatchCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("watch")
	builder.WithShort("Watch the MCP daemon for events and process the feed")
	help := clipkg.DynamicHelpBuilder("Watch the MCP daemon for events and process the feed")
	help.WithDescriptionLines("Connects to the local MCP daemon over TCP, subscribes to notifications/event,")
	help.WithDescriptionLines("and on action.required ticks the agent feed inbox for --agent-id.")
	help.WithDescriptionLines("Optionally appends peer_ack lines with --auto-ack.")
	help.WithDescriptionLines("Runs until interrupted (long-lived; prefer a dedicated terminal or supervisor).")
	help.AddExample("Watch operator seat inbox", "%s feed watch --agent-id peer-tpm-01 --persona-ref PER-DEFAULT-OPERATOR")
	help.AddExample("Watch and auto-ack", "%s feed watch --agent-id peer-tpm-01 --auto-ack")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("tcp", "", "127.0.0.1:8443", "MCP daemon TCP address")
	builder.AddStringFlag("agent-id", "", "", "Swarm seat id whose inbox to process")
	builder.AddStringFlag("persona-ref", "", "PER-DEFAULT-OPERATOR", "Kernel persona id (PER-*) used when auto-acking")
	builder.AddBoolFlag("auto-ack", "", false, "Automatically peer-ack unacked inbox events")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down", "format", "output"})
	cmd := builder.Build()
	return cmd
}
