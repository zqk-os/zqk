package feed

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewWakeCmd creates zqk feed wake.
func NewWakeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedWakeCommandBuilder()
	cmd.RunE = runFeedWake
	return cmd
}

func runFeedWake(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == "" {
			return errfmt.Errorf("project root not found")
		}
		var flags clipkg.FlagBag
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

		// By policy, "wake" is doer-safe (notify-only). We do NOT check DenyPeerSteerIfDoer here.

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     message,
			AgentID:     agentID,
			ToAgentID:   toAgentID,
			Sender:      agentfeed.FeedSenderHumanSteer, // Re-use the existing sender or similar
			EventType:   agentfeed.FeedEventTypeWake,
			SelfACK:     !noAck,
		})
		if err != nil {
			return errfmt.Newf("feed wake").Wrap(err)
		}
		if want := strings.TrimSpace(feedID); want != "" && res.FeedID != "" && want != res.FeedID {
			return errfmt.Errorf("lite feed_id %q does not match --feed-id %q (rematerialize agent_chat_channel)", res.FeedID, want)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed wake appended").
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
				return errfmt.Newf("feed wake: register peer-ack await").Wrap(aerr)
			}
			awaitID = aw.ID
			logging.Fluent(logger).Info("peer-ack await registered").
				String("await_id", aw.ID).
				String("event_id", res.EventID).
				Log()
		}

		var wake agentfeed.PeerWakeResult
		var wakePtr *agentfeed.PeerWakeResult
		mcpSubscribers := -1
		mcpSubscribersProbed := false
		mcpIPCDelivered := false
		if !zqkenv.IsCommunityEdition && !noWake && agentfeed.ShouldWakePeer(res.DeliveryMode) {
			if res.DeliveryMode == datacell.DeliveryModeNotify {
				probe := mcp.ProbeFeedSteerMCPWake(cmd.Context(), "", message, agentID, res.EventID, logger)
				mcpIPCDelivered = probe.IPCDelivered
				mcpSubscribersProbed = probe.SubscribersProbed
				mcpSubscribers = probe.SubscriberCount
				if probe.PublishErr == nil {
					logging.Fluent(logger).Info("feed wake MCP daemon IPC wake delivered").
						String("event_id", res.EventID).
						Log()
				} else {
					logging.Fluent(logger).Warn("feed wake MCP daemon IPC publish failed; continuing with wake script").
						String("error", probe.PublishErr.Error()).
						Log()
				}
				if probe.QueryErr != nil {
					logging.Fluent(logger).Warn("feed wake MCP subscriber count query failed").
						String("error", probe.QueryErr.Error()).
						Log()
				} else if probe.SubscribersProbed {
					logging.Fluent(logger).Info("feed wake MCP subscriber count").
						Int("subscriber_count", probe.SubscriberCount).
						Log()
				}
			}

			wake = agentfeed.WakePeerOpts(cmd.Context(), agentfeed.WakePeerOptions{
				ProjectRoot:          root,
				Message:              message,
				InReplyTo:            res.EventID,
				FromAgentID:          agentID,
				DeliveryMode:         res.DeliveryMode,
				ToAgentID:            toAgentID,
				MCPIPCDelivered:      mcpIPCDelivered,
				MCPSubscribersProbed: mcpSubscribersProbed,
				MCPSubscriberCount:   mcpSubscribers,
			})
			wakePtr = &wake
			if unrepaired, reason, detail := agentfeed.IsUnrepairedWake(wake); unrepaired {
				_ = agentfeed.AlertUnrepairedWake(res.EventID, detail, reason)
				logging.Fluent(logger).Warn("feed wake peer wake unrepaired (event still appended; human alerted)").
					String("reason", string(reason)).
					String("detail", detail).
					String("transport", wake.Transport).
					Path(wake.Script).
					Log()
			} else if wake.Attempted {
				logging.Fluent(logger).Info("feed wake peer wake attempted").
					Path(wake.Script).
					String("transport", wake.Transport).
					String("delivery_receipt", fmt.Sprintf("%v", wake.DeliveryReceipt)).
					Bool("live", wake.Live).
					Log()
			}
		} else if zqkenv.IsCommunityEdition && !noWake && agentfeed.ShouldWakePeer(res.DeliveryMode) {
			logging.Fluent(logger).Info("feed wake: community edition skips Studio mesh wake membrane").
				String("delivery_mode", res.DeliveryMode).
				Log()
		}

		out := feedResult(cmd, res, wakePtr)
		if awaitID != "" {
			out["peer_ack_await_id"] = awaitID
			out["await_peer_ack"] = true
		}
		if mcpSubscribers >= 0 {
			out["mcp_subscribers"] = mcpSubscribers
		}
		if wakePtr != nil && wakePtr.Transport != "" {
			out["peer_wake_transport"] = wakePtr.Transport
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, nil)
}
