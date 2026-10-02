package feed

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// NewAckCmd creates zqk feed ack (peer cognitive acknowledgment).
func NewAckCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewAckCommandBuilder()
	cmd.RunE = runFeedAck
	return cmd
}

func runFeedAck(cmd *cobra.Command, args []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		inReplyTo := strings.TrimSpace(flags.String(cmd, "in-reply-to"))
		agentID := strings.TrimSpace(flags.String(cmd, "agent-id"))
		personaRef := strings.TrimSpace(flags.String(cmd, "persona-ref"))
		summary := flags.String(cmd, "summary")
		if err := flags.Err(); err != nil {
			return err
		}
		if inReplyTo == "" {
			return errfmt.Errorf("--in-reply-to is required")
		}
		if agentID == "" {
			return errfmt.Errorf("--agent-id is required")
		}
		if personaRef == "" {
			return errfmt.Errorf("--persona-ref is required")
		}

		if _, err := resolveAndValidatePersona(proc, personaRef, "ack"); err != nil {
			return err
		}

		if err := agentfeed.AuthorizePeerAck(root, agentID, inReplyTo); err != nil {
			return err
		}

		res, err := agentfeed.AppendPeerAck(root, agentID, personaRef, inReplyTo, summary)
		if err != nil {
			return errfmt.Newf("feed ack").Wrap(err)
		}

		completed, cerr := agentfeed.CompletePeerAckAwaits(root, inReplyTo, agentID)
		if cerr != nil {
			logging.Fluent(logging.GetLoggerFromProfile(proc.Context().Profile)).Warn("peer-ack await callback fire had errors").
				String("error", cerr.Error()).
				String("in_reply_to", inReplyTo).
				Log()
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed peer_ack appended").
			Path(res.EventPath).
			String("event_id", res.EventID).
			String("in_reply_to", inReplyTo).
			AgentID(agentID).
			PersonaRef(personaRef).
			Int("awaits_completed", len(completed)).
			Log()

		out := newPersonaFeedResult(cmd, res, personaRef, agentID)
		out["in_reply_to"] = inReplyTo
		out[objects.FieldKeyEventType] = agentfeed.FeedEventTypePeerAck
		if len(completed) > 0 {
			ids := make([]string, 0, len(completed))
			for _, a := range completed {
				ids = append(ids, a.ID)
			}
			out["awaits_completed"] = ids
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
