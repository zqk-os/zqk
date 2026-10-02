package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/authcred"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewSteerCmd creates zqk feed steer.
func NewSteerCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSteerCommandBuilder()
	cmd.RunE = runFeedSteer
	return cmd
}

func runFeedSteer(cmd *cobra.Command, _ []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		message := flags.String(cmd, "message")
		agentID := flags.String(cmd, "agent-id")
		feedID := flags.String(cmd, "feed-id")
		noAck := flags.Bool(cmd, "no-ack")
		noWake := flags.Bool(cmd, "no-wake")
		toAgentID := flags.String(cmd, "to-agent-id")
		awaitPeerAck := flags.Bool(cmd, "await-peer-ack")
		if err := flags.Err(); err != nil {
			return err
		}
		// Directed routing without await is fire-and-forget.
		if err := agentfeed.EnforceDirectedHourglass(toAgentID, awaitPeerAck); err != nil {
			return err
		}

		// POL-AGENT-PLANNER-DOER-001: directed peer steer is planner-lane only.
		if err := authcred.DenyPeerSteerIfDoer(proc.SecurityContext(), toAgentID); err != nil {
			return err
		}

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     message,
			AgentID:     agentID,
			ToAgentID:   toAgentID,
			Sender:      agentfeed.FeedSenderHumanSteer,
			EventType:   agentfeed.FeedEventTypeSteering,
			SelfACK:     !noAck,
		})
		if err != nil {
			return errfmt.Newf("feed steer").Wrap(err)
		}
		if want := strings.TrimSpace(feedID); want != "" && res.FeedID != "" && want != res.FeedID {
			return errfmt.Errorf("lite feed_id %q does not match --feed-id %q (rematerialize agent_chat_channel)", res.FeedID, want)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed steer appended").
			Path(res.EventPath).
			String("feed_id", res.FeedID).
			String("event_id", res.EventID).
			String("delivery_mode", res.DeliveryMode).
			Log()

		var awaitID string
		if awaitPeerAck {
			aw, aerr := agentfeed.RegisterPeerAckAwait(root, agentfeed.PeerAckAwaitInput{
				EventID:     res.EventID,
				FromAgentID: agentID,
				ToAgentID:   toAgentID,
				Action:      agentfeed.AwaitActionWake,
				WakeMessage: agentfeed.PeerAckPasteStub(res.EventID),
			})
			if aerr != nil {
				return errfmt.Newf("feed steer: register peer-ack await").Wrap(aerr)
			}
			awaitID = aw.ID
			logging.Fluent(logger).Info("peer-ack await registered").
				String("await_id", aw.ID).
				String("event_id", res.EventID).
				Log()
		}

		wakePtr, mcpSubscribers := dispatchPeerWake(
			cmd.Context(),
			logger,
			root,
			message,
			res,
			agentID,
			toAgentID,
			noWake,
			"steer",
		)
		return formatPeerWakeOutput(cmd, res, wakePtr, awaitID, mcpSubscribers)
	})(cmd, nil)
}
