package agentfeed

import (
	"errors"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// ErrPeerAckSeatMismatch is returned when --agent-id does not match the
// parent event's to_agent_id (cross-seat impersonation).
var ErrPeerAckSeatMismatch = errors.New("peer_ack agent_id does not match parent to_agent_id (seat impersonation denied)")

// AuthorizePeerAck fails closed when the parent steer is directed
// (to_agent_id set) and ackingAgentID is not that seat (aliases denied).
// Undirected parents (no to_agent_id) remain ackable by any seat.
// TRACK: BLI-REDACTED — harden further with session/PID binding (agent_id stamp alone is honor+gate).
func AuthorizePeerAck(projectRoot, ackingAgentID, inReplyTo string) error {
	acking := strings.TrimSpace(ackingAgentID)
	parentID := strings.TrimSpace(inReplyTo)
	if acking == "" {
		return errfmt.Errorf("agent_id is required for peer_ack")
	}
	if parentID == "" {
		return errfmt.Errorf("in_reply_to is required for peer_ack")
	}
	parent, ok, err := LookupFeedEvent(projectRoot, parentID)
	if err != nil {
		return err
	}
	if !ok {
		// Unknown parent: still allow append (legacy rows / races) but do not
		// open a hole for known directed steers.
		return nil
	}
	to := strings.TrimSpace(objects.StringField(parent, JSONFieldToAgentID))
	if to == "" {
		return nil
	}
	if to == acking {
		return nil
	}
	return errfmt.Newf(
		"peer_ack denied: parent %s is addressed to %q but ack stamped agent_id=%q — only the addressed seat may peer_ack (aliases denied)",
		parentID, to, acking,
	).Wrap(ErrPeerAckSeatMismatch)
}

// LookupFeedEvent returns the first JSONL row with matching event_id.
func LookupFeedEvent(projectRoot, eventID string) (map[string]any, bool, error) {
	eid := strings.TrimSpace(eventID)
	if eid == "" {
		return nil, false, nil
	}
	cfg, err := datacell.ReadAgentChatChannelConfig(projectRoot)
	if err != nil {
		return nil, false, errfmt.Errorf("read agent chat channel config: %w", err)
	}
	path := datacell.EffectiveAgentChatChannelEventsJSONLPath(projectRoot, cfg)
	events, err := readJSONLEvents(path)
	if err != nil {
		return nil, false, err
	}
	for _, ev := range events {
		if objects.StringField(ev, JSONFieldEventID) == eid {
			return ev, true, nil
		}
	}
	return nil, false, nil
}

// PeerAckSatisfiesParent reports whether a peer_ack from ackAgent clears a
// parent event for correspondence / await completion.
func PeerAckSatisfiesParent(parent map[string]any, ackAgentID string) bool {
	to := strings.TrimSpace(objects.StringField(parent, JSONFieldToAgentID))
	ack := strings.TrimSpace(ackAgentID)
	if ack == "" || ack == FeedKernelAgentID {
		return false
	}
	if to == "" {
		return true
	}
	return seatMatchesAgent(Seat{AgentID: to}, ack)
}
