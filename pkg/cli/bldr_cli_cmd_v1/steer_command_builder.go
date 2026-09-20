package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSteerCommandBuilder creates a new steer command
func NewSteerCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("steer")
	builder.WithShort("Append a steering event to the agent chat feed")
	help := clipkg.DynamicHelpBuilder("Append a steering event to the agent chat feed")
	help.WithDescriptionLines("Appends a typed steering event (and optional kernel ACK) to the agent chat")
	help.WithDescriptionLines("channel JSONL. Requires the lite channel enabled with delivery_mode other than off.")
	help.WithDescriptionLines("When delivery_mode is notify or paste, also attempts a peer wake unless --no-wake.")
	help.WithDescriptionLines("POL-AGENT-ORCH-HOURGLASS-001: --to-agent-id requires --await-peer-ack (fail-closed).")
	help.AddExample("Steer a peer via the feed", "%s feed steer --message \"ATTN peer: verify work item\"")
	help.AddExample("Stamp with explicit agent id", "%s feed steer --agent-id coordinator --message \"ACCEPT item\"")
	help.AddExample("Ring caller when peer feed-acks (callback ringer)", "%s feed steer --agent-id peer-tpm-01 --to-agent-id peer-agent-01 --await-peer-ack --message \"ATTN AGY: start ATK\"")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddStringFlag("message", "m", "", "Steering message body")
	builder.AddStringFlag("agent-id", "", "human", "Caller agent id stamp")
	builder.AddStringFlag("feed-id", "", "", "Fail if the lite feed_id differs from this value")
	builder.AddBoolFlag("no-ack", "", false, "Do not append a kernel self-ACK line")
	builder.AddBoolFlag("no-wake", "", false, "Do not wake the peer (stamp the feed only)")
	builder.AddStringFlag("to-agent-id", "", "", "Peer seat id for inbox routing (requires --await-peer-ack)")
	builder.AddBoolFlag("await-peer-ack", "", false, "Required with --to-agent-id: register peer-ack await callback (POL-AGENT-ORCH-HOURGLASS-001)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down"})
	cmd := builder.Build()
	return cmd
}
