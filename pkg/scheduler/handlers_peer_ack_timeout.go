package scheduler

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/agentfeed"
	"github.com/lanceman/zqk/pkg/logging"
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
			msg := "ATTN " + ex.FromAgentID + " — peer-ack timeout expired for " + ex.EventID
			if ex.WakeMessage != "" {
				msg = msg + ": " + ex.WakeMessage
			}
			_, _ = agentfeed.AppendEvent(agentfeed.AppendEventInput{
				ProjectRoot: h.projectRoot,
				Message:     msg,
				AgentID:     agentfeed.FeedKernelAgentID,
				ToAgentID:   ex.FromAgentID,
				Sender:      agentfeed.FeedSenderMeshStatus,
				EventType:   agentfeed.FeedEventTypeWake,
			})
		}
	}

	if len(expired) > 0 && h.logger != nil {
		SLog(h.logger).Info("Expired stale peer ack awaits").Int("count", len(expired)).Log()
	}

	return nil
}

// Drain clears context on shutdown
func (h *PeerAckTimeoutAuditHandler) Drain() {}
