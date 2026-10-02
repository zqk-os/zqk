package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewPendingCmd creates zqk feed pending (diagnostic inbox/outbox).
func NewPendingCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewPendingCommandBuilder()
	cmd.RunE = runFeedPending
	return cmd
}

func runFeedPending(cmd *cobra.Command, args []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		agentID := strings.TrimSpace(flags.String(cmd, "agent-id"))
		personaRef := flags.String(cmd, "persona-ref")
		limit := flags.Int(cmd, "limit")
		if err := flags.Err(); err != nil {
			return err
		}
		if agentID == "" {
			return errfmt.Errorf("--agent-id is required")
		}
		snap, err := agentfeed.LoadCorrespondence(root, agentfeed.Seat{
			AgentID:    agentID,
			PersonaRef: strings.TrimSpace(personaRef),
		}, limit)
		if err != nil {
			return errfmt.Newf("feed pending").Wrap(err)
		}
		out := map[string]any{
			objects.FieldKeyStatus:          "success",
			objects.FieldKeyNote:            "diagnostic only — prefer workflow whats-next correspondence",
			agentfeed.JSONFieldPersonaID:    snap.PersonaID,
			agentfeed.JSONFieldAgentID:      snap.AgentID,
			agentfeed.JSONFieldInboxUnacked: snap.InboxUnacked,
			agentfeed.JSONFieldOutboxAwait:  snap.OutboxAwaitingPeerAck,
			objects.FieldKeyNextActionHint:  snap.NextActionHint,
			agentfeed.JSONFieldSkipReason:   snap.SkipReason,
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
