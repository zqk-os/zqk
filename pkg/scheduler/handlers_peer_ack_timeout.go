package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
	"github.com/zqk-os/zqk/pkg/idebridge"
	"github.com/zqk-os/zqk/pkg/logging"
)

// DefaultPeerAckAwaitMaxAge is how long an open peer-ack await may sit before expiry audit.
const DefaultPeerAckAwaitMaxAge = 30 * time.Minute

// PeerAckTimeoutAuditHandler checks for timed out peer-ack awaits and expires them.
type PeerAckTimeoutAuditHandler struct {
	projectRoot string
	logger      logging.Logger
	maxAge      time.Duration
}

// NewPeerAckTimeoutAuditHandler creates a new handler with DefaultPeerAckAwaitMaxAge.
func NewPeerAckTimeoutAuditHandler(projectRoot string, logger logging.Logger) JobHandler {
	return &PeerAckTimeoutAuditHandler{
		projectRoot: projectRoot,
		logger:      logger,
		maxAge:      DefaultPeerAckAwaitMaxAge,
	}
}

// NewPeerAckTimeoutAuditHandlerWithMaxAge creates a handler with an explicit max age (tests / overrides).
func NewPeerAckTimeoutAuditHandlerWithMaxAge(projectRoot string, logger logging.Logger, maxAge time.Duration) JobHandler {
	if maxAge <= 0 {
		maxAge = DefaultPeerAckAwaitMaxAge
	}
	return &PeerAckTimeoutAuditHandler{
		projectRoot: projectRoot,
		logger:      logger,
		maxAge:      maxAge,
	}
}

// Execute performs the timeout audit.
func (h *PeerAckTimeoutAuditHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	expired, err := agentfeed.ExpirePeerAckAwaits(h.projectRoot, h.maxAge)
	if err != nil {
		if h.logger != nil {
			SLog(h.logger).Warn("Failed to expire peer ack awaits").WithError(err).Log()
		}
		return err
	}

	// Group wake-action expiries by await author so one audit tick cannot
	// flood Composer with N identical ATTN injects (2026-11-05 spam).
	// TRACK: BLI-1785905541906569000-074e24d7 — peer_ack_timeout batch wake
	type wakeBatch struct {
		eventIDs []string
	}
	byCaller := map[string]*wakeBatch{}

	for _, ex := range expired {
		ackAgent := ex.ToAgentID
		if ackAgent == "" {
			ackAgent = "kernel-timeout"
		}

		_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: h.projectRoot,
			Message:     "PEER_ACK_TIMEOUT — auto-clearing stale wait",
			AgentID:     ackAgent,
			PersonaRef:  "system-timeout",
			Sender:      agentfeed.FeedSenderPeerAck,
			EventType:   agentfeed.FeedEventTypePeerAck,
			InReplyTo:   ex.EventID,
			SelfACK:     false,
		})

		if ex.Action == agentfeed.AwaitActionWake && ex.FromAgentID != "" {
			b := byCaller[ex.FromAgentID]
			if b == nil {
				b = &wakeBatch{}
				byCaller[ex.FromAgentID] = b
			}
			b.eventIDs = append(b.eventIDs, ex.EventID)
		}
	}

	for fromID, batch := range byCaller {
		n := len(batch.eventIDs)
		sample := batch.eventIDs[0]
		var msg string
		if n == 1 {
			// Do NOT append stored WakeMessage — register-time stub is
			// "peer_ack received", which makes timeout ATTNs self-contradictory.
			msg = "ATTN " + fromID + " — peer-ack timeout expired for " + sample + " (1 await; check whats-next / open hourglasses)"
		} else {
			msg = "ATTN " + fromID + " — peer-ack timeout expired for " + fmt.Sprintf("%d", n) + " awaits (e.g. " + sample + "); ack inbox once, do not remint steers. Check whats-next."
		}
		_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
			ProjectRoot: h.projectRoot,
			Message:     msg,
			AgentID:     agentfeed.FeedKernelAgentID,
			ToAgentID:   fromID,
			Sender:      agentfeed.FeedSenderMeshStatus,
			EventType:   agentfeed.FeedEventTypeWake,
		})
		// One membrane wake + one IDE inject per caller per audit tick.
		// TRACK: POL-AGENT-MESH-WAKE-001 / POL-AGENT-ORCH-HOURGLASS-001
		wakeRes := agentfeed.WakePeerOpts(ctx, agentfeed.WakePeerOptions{
			ProjectRoot:  h.projectRoot,
			Message:      msg,
			ToAgentID:    fromID,
			FromAgentID:  "hourglass-peer-ack-timeout",
			DeliveryMode: "notify",
		})
		if !wakeRes.IdeBridgeQueued {
			_ = idebridge.QueueWakeAttn(h.projectRoot, msg)
		}
	}

	if len(expired) > 0 && h.logger != nil {
		SLog(h.logger).Info("Expired stale peer ack awaits").Int("count", len(expired)).Log()
	}

	return nil
}

// Drain clears context on shutdown
func (h *PeerAckTimeoutAuditHandler) Drain() {}
