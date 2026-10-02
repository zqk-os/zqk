package feed

import (
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
		p, err := parsePeerEventFlags(cmd, flags)
		if err != nil {
			return err
		}

		// POL-AGENT-PLANNER-DOER-001: directed peer steer is planner-lane only.
		if err := authcred.DenyPeerSteerIfDoer(proc.SecurityContext(), p.toAgentID); err != nil {
			return err
		}

		res, err := agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: root,
			Message:     p.message,
			AgentID:     p.agentID,
			ToAgentID:   p.toAgentID,
			Sender:      agentfeed.FeedSenderHumanSteer,
			EventType:   agentfeed.FeedEventTypeSteering,
			SelfACK:     !p.noAck,
		})
		if err != nil {
			return errfmt.Newf("feed steer").Wrap(err)
		}
		if err := validateLiteFeedID(res.FeedID, p.feedID); err != nil {
			return err
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed steer appended").
			Path(res.EventPath).
			String("feed_id", res.FeedID).
			String("event_id", res.EventID).
			String("delivery_mode", res.DeliveryMode).
			Log()

		var awaitID string
		if p.awaitPeerAck {
			awaitID, err = registerPeerAckAwaitHelper(logger, root, res, p.agentID, p.toAgentID, agentfeed.AwaitActionWake, "steer")
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
			"steer",
		)
		return formatPeerWakeOutput(cmd, res, wakePtr, awaitID, mcpSubscribers)
	})(cmd, nil)
}
