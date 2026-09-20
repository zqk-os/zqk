package mcp

import (
	"context"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
)

// DefaultFeedSteerMCPProbeTimeout bounds ActionRequired publish + events/list
// probes used by feed steer / feed serve (CRIT-COMMS-003).
const DefaultFeedSteerMCPProbeTimeout = 1 * time.Second

// ResolveDaemonTCP returns DefaultDaemonTCP when raw is empty/whitespace.
func ResolveDaemonTCP(raw string) string {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return DefaultDaemonTCP
	}
	return addr
}

// FeedSteerMCPWakeResult is the MCP interrupt evidence attached to PeerWakeOptions.
type FeedSteerMCPWakeResult struct {
	IPCDelivered      bool
	SubscriberCount   int
	SubscribersProbed bool
	PublishErr        error
	QueryErr          error
}

// ProbeFeedSteerMCPWake publishes ActionRequired and queries subscriber count.
// TRACK: CRIT-COMMS-003 / BLI-COMMS-TPM-LIVE-WAKE-001 — shared by CLI steer and HTTP serve.
func ProbeFeedSteerMCPWake(ctx context.Context, tcpAddr, message, agentID, eventID string, logger logging.Logger) FeedSteerMCPWakeResult {
	pd := NewProxyDaemon(ResolveDaemonTCP(tcpAddr), logger)
	out := FeedSteerMCPWakeResult{SubscriberCount: -1}

	pCtx, pCancel := context.WithTimeout(ctx, DefaultFeedSteerMCPProbeTimeout)
	out.PublishErr = pd.PublishDaemonEvent(pCtx, message, agentID, eventID)
	pCancel()
	if out.PublishErr == nil {
		out.IPCDelivered = true
	}

	qCtx, qCancel := context.WithTimeout(ctx, DefaultFeedSteerMCPProbeTimeout)
	n, qErr := pd.QueryEventsSubscriberCount(qCtx)
	qCancel()
	out.QueryErr = qErr
	if qErr == nil {
		out.SubscriberCount = n
		out.SubscribersProbed = true
	}
	return out
}
