package feed

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/agentfeed"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// wakeStubBodyPrefix marks inbox entries that carry only a wake stub, so operators
// can tell them apart from steering events with real correspondence.
const wakeStubBodyPrefix = "[wake/notify] "

// NewWatchCmd creates the feed watch command.
func NewWatchCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewWatchCommandBuilder()
	cmd.RunE = runFeedWatch
	return cmd
}

func runFeedWatch(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		root := proc.ProjectRoot()
		if root == "" {
			return errfmt.Errorf("project root not found")
		}

		var flags clipkg.FlagBag
		agentID := strings.TrimSpace(flags.String(cmd, "agent-id"))
		personaRef := strings.TrimSpace(flags.String(cmd, "persona-ref"))
		tcpAddr := resolveFeedMCPTCP(flags.String(cmd, "tcp"))
		autoAck := flags.Bool(cmd, "auto-ack")

		if err := flags.Err(); err != nil {
			return err
		}
		if agentID == "" {
			return errfmt.Errorf("--agent-id is required")
		}

		logger := logging.GetLoggerFromProfile(proc.Context().Profile)

		for {
			runSubscriberLoop(cmd.Context(), logger, tcpAddr, root, agentID, personaRef, autoAck)

			select {
			case <-cmd.Context().Done():
				return nil
			case <-time.After(5 * time.Second):
			}
		}
	})(cmd, nil)
}

func runSubscriberLoop(ctx context.Context, logger logging.Logger, tcpAddr, root, agentID, personaRef string, autoAck bool) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", tcpAddr)
	if err != nil {
		logging.Fluent(logger).Warn("MCP daemon not running").WithError(errfmt.Newf("dial").Wrap(err)).Addr(tcpAddr).Log()
		return
	}
	defer conn.Close()

	logging.Fluent(logger).Info("Connected to MCP daemon. Initializing...").Log()

	initMsg := map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     1,
		objects.FieldKeyMethod: "initialize",
		"params": map[string]any{
			"protocolVersion":            "2024-11-05",
			objects.FieldKeyCapabilities: map[string]any{},
			"clientInfo":                 map[string]any{objects.FieldKeyName: "local-subscriber-stub", objects.FieldKeyVersion: "1.0"},
		},
	}
	initBytes, _ := json.Marshal(initMsg)
	_, _ = conn.Write(append(initBytes, '\n'))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		var resp map[string]any
		if err := json.Unmarshal(line, &resp); err == nil {
			if id, ok := resp[objects.FieldKeyID].(float64); ok && id == 1 {
				break
			}
		}
	}

	initializedMsg := map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyMethod: "notifications/initialized",
	}
	initdBytes, _ := json.Marshal(initializedMsg)
	_, _ = conn.Write(append(initdBytes, '\n'))

	logging.Fluent(logger).Info("Subscribing to events...").Log()
	subMsg := map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyID:     2,
		objects.FieldKeyMethod: "events/subscribe",
		"params":               map[string]any{},
	}
	subBytes, _ := json.Marshal(subMsg)
	_, _ = conn.Write(append(subBytes, '\n'))

	logging.Fluent(logger).Info("Listening for events...").Log()

	processFeed(logger, root, agentID, personaRef, autoAck)

	for scanner.Scan() {
		line := scanner.Bytes()
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if method, ok := msg[objects.FieldKeyMethod].(string); ok && method == "notifications/event" {
			params, _ := msg["params"].(map[string]any)
			event, _ := params["event"].(map[string]any)
			eventType, _ := event[objects.FieldKeyType].(string)

			if eventType == "action.required" || eventType == "action_required" {
				logging.Fluent(logger).Info("Received action.required event. Processing feed...").Log()
				processFeed(logger, root, agentID, personaRef, autoAck)
			}
		}
	}
}

func processFeed(logger logging.Logger, root, agentID, personaRef string, autoAck bool) {
	snap, err := agentfeed.LoadCorrespondence(root, agentfeed.Seat{
		AgentID:    agentID,
		PersonaRef: personaRef,
	}, 100)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to load correspondence").WithError(errfmt.Newf("load").Wrap(err)).Log()
		return
	}

	inboxCount := len(snap.InboxUnacked)
	outboxCount := len(snap.OutboxAwaitingPeerAck)
	logging.Fluent(logger).Info("Feed pending status").
		Int("inbox", inboxCount).
		Int("outbox", outboxCount).
		String("hint", snap.NextActionHint).
		Log()

	if inboxCount == 0 {
		return
	}

	// Cursor TPM: ActionRequired/toast does not start a turn. Queue composer inject
	// only for work steers — parked peer_ack / hourglass callbacks are ack theatre.
	if agentID == "cursor-composer" || strings.HasPrefix(agentID, "cursor-") {
		if first, ok := agentfeed.FirstComposerHourglassItem(snap.InboxUnacked); ok {
			attn := agentfeed.TPMComposerAttnCue(first, agentID)
			if idebridge.QueueWakeAttn(root, attn) {
				logging.Fluent(logger).Info("Queued zqk.wake.attn for Composer inject").String(agentfeed.JSONFieldEventID, first.EventID).Log()
			}
		} else {
			logging.Fluent(logger).Info("Skipped zqk.wake.attn — inbox is ack theatre only").
				Int("inbox", inboxCount).
				Log()
		}
	}

	for _, x := range snap.InboxUnacked {
		body := x.Message
		if body == "" {
			body = x.Summary
		}

		if x.EventType == agentfeed.FeedEventTypeWake {
			body = wakeStubBodyPrefix + x.Summary
		}

		logging.Fluent(logger).Info("INBOX").
			String(agentfeed.JSONFieldEventID, x.EventID).
			String(agentfeed.JSONFieldMessage, body).
			Log()
	}

	if autoAck {
		for _, x := range snap.InboxUnacked {
			if x.EventID == "" {
				continue
			}
			_, aErr := agentfeed.AppendPeerAck(root, agentID, personaRef, x.EventID, "PEER_ACK watch received "+x.EventID+" — auto-acked")
			if aErr != nil {
				logging.Fluent(logger).Warn("Failed to auto-ack").WithError(errfmt.Newf("auto-ack").Wrap(aErr)).String("event_id", x.EventID).Log()
			} else {
				logging.Fluent(logger).Info("Auto-acked event").String("event_id", x.EventID).Log()
			}
		}
	}
}
