package feed

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/agentfeed/bridge"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// NewBridgeIngestCmd creates zqk feed bridge-ingest.
func NewBridgeIngestCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewBridgeIngestCommandBuilder()
	cmd.RunE = runFeedBridgeIngest
	return cmd
}

func runFeedBridgeIngest(cmd *cobra.Command, args []string) error {
	return withFeedRoot(func(cmd *cobra.Command, proc *cli.Processor, root string, flags *clipkg.FlagBag) error {
		channel := flags.String(cmd, "channel")
		payloadFile := flags.String(cmd, "payload-file")
		payloadInline := flags.String(cmd, "payload")
		agentID := flags.String(cmd, "agent-id")
		toAgentID := flags.String(cmd, "to-agent-id")
		feedID := flags.String(cmd, "feed-id")
		noAck := flags.Bool(cmd, "no-ack")
		noWake := flags.Bool(cmd, "no-wake")
		if err := flags.Err(); err != nil {
			return err
		}

		raw, err := readBridgePayload(payloadFile, payloadInline)
		if err != nil {
			return err
		}
		msg, err := bridge.Parse(channel, raw)
		if err != nil {
			return errfmt.Newf("feed bridge-ingest parse").Wrap(err)
		}
		if id := strings.TrimSpace(agentID); id != "" {
			msg.AgentID = id
		}
		if id := strings.TrimSpace(toAgentID); id != "" {
			msg.ToAgentID = id
		}

		in, err := bridge.ToSteerInput(root, msg)
		if err != nil {
			return errfmt.Newf("feed bridge-ingest map").Wrap(err)
		}
		in.SelfACK = !noAck

		res, err := agentfeed.AppendEvent(in)
		if err != nil {
			return errfmt.Newf("feed bridge-ingest").Wrap(err)
		}
		if want := strings.TrimSpace(feedID); want != "" && res.FeedID != "" && want != res.FeedID {
			return errfmt.Errorf("lite feed_id %q does not match --feed-id %q (rematerialize agent_chat_channel)", res.FeedID, want)
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)
		logging.Fluent(logger).Info("feed bridge-ingest appended").
			Path(res.EventPath).
			String("channel", string(msg.Channel)).
			String("feed_id", res.FeedID).
			String("event_id", res.EventID).
			String("delivery_mode", res.DeliveryMode).
			Log()

		var wakePtr *agentfeed.PeerWakeResult
		if !zqkenv.IsCommunityEdition && !noWake && agentfeed.ShouldWakePeer(res.DeliveryMode) {
			wake := agentfeed.WakePeerOpts(cmd.Context(), agentfeed.WakePeerOptions{
				ProjectRoot:  root,
				Message:      in.Message,
				InReplyTo:    res.EventID,
				FromAgentID:  in.AgentID,
				DeliveryMode: res.DeliveryMode,
				ToAgentID:    in.ToAgentID,
			})
			wakePtr = &wake
			if unrepaired, reason, detail := agentfeed.IsUnrepairedWake(wake); unrepaired {
				_ = agentfeed.AlertUnrepairedWake(res.EventID, detail, reason)
				logging.Fluent(logger).Warn("feed bridge-ingest peer wake unrepaired (event still appended; human alerted)").
					String("reason", string(reason)).
					String("detail", detail).
					Path(wake.Script).
					Log()
			}
		}

		out := feedResult(cmd, res, wakePtr)
		out["channel"] = string(msg.Channel)
		if msg.ExternalID != "" {
			out["external_id"] = msg.ExternalID
		}
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}

func readBridgePayload(payloadFile, payloadInline string) ([]byte, error) {
	file := strings.TrimSpace(payloadFile)
	inline := strings.TrimSpace(payloadInline)
	switch {
	case file != "" && inline != "":
		return nil, errfmt.Errorf("provide only one of --payload-file or --payload")
	case file != "":
		b, err := fileutil.ReadFile(file)
		if err != nil {
			return nil, errfmt.Newf("read --payload-file").Wrap(err)
		}
		return b, nil
	case inline != "":
		return []byte(inline), nil
	default:
		return nil, errfmt.Errorf("missing required parameter: --payload-file or --payload")
	}
}
