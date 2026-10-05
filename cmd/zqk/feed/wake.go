package feed

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewWakeCmd creates zqk feed wake.
func NewWakeCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewFeedWakeCommandBuilder()
	cmd.RunE = runFeedWake
	return cmd
}

func runFeedWake(cmd *cobra.Command, _ []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		p, err := parsePeerEventFlags(cmd, flags)
		if err != nil {
			return err
		}

		// By policy, "wake" is doer-safe (notify-only). We do NOT check DenyPeerSteerIfDoer here.

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     p.message,
			AgentID:     p.agentID,
			ToAgentID:   p.toAgentID,
			Sender:      agentfeed.FeedSenderHumanSteer, // Re-use the existing sender or similar
			EventType:   agentfeed.FeedEventTypeWake,
			SelfACK:     !p.noAck,
		})
		if err != nil {
			return errfmt.Newf("feed wake").Wrap(err)
		}
		if err := validateLiteFeedID(res.FeedID, p.feedID); err != nil {
			return err
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed wake appended").
			Path(res.EventPath).
			String("feed_id", res.FeedID).
			String("event_id", res.EventID).
			String("delivery_mode", res.DeliveryMode).
			Log()

		var awaitID string
		if p.awaitPeerAck {
			awaitID, err = registerPeerAckAwaitHelper(logger, root, res, p.agentID, p.toAgentID, agentfeed.AwaitActionWake, "wake")
			if err != nil {
				return err
			}
		}

		wakePtr, mcpSubscribers := dispatchPeerWake(
			cmd.Context(),
			logger,
			root,
			p.message,
			res,
			p.agentID,
			p.toAgentID,
			p.noWake,
			"wake",
		)
		return formatPeerWakeOutput(cmd, res, wakePtr, awaitID, mcpSubscribers)
	})(cmd, nil)
}
