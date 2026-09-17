package agentfeed

import (
	"bufio"
	"encoding/json"
	"strings"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// Seat identifies a swarm agent for inbox/outbox correspondence.
type Seat struct {
	AgentID    string
	PersonaRef string
	// RoleHints match ATTN lines (e.g. "agy", "tpm") when to_agent_id is unset.
	RoleHints []string
}

// CorrespondenceItem is a feed event summarized for whats-next / pending.
type CorrespondenceItem struct {
	EventID             string `json:"event_id"`
	Timestamp           string `json:"timestamp,omitempty"`
	Summary             string `json:"summary,omitempty"`
	Message             string `json:"message,omitempty"` // full body when available (seat-worker / COMMS)
	EventType           string `json:"event_type,omitempty"`
	FromAgentID         string `json:"from_agent_id,omitempty"`
	ToAgentID           string `json:"to_agent_id,omitempty"`
	HasDeliveryReceipt  bool   `json:"has_delivery_receipt"`
	DeliveryReceiptOnly bool   `json:"delivery_receipt,omitempty"` // alias for inbox: delivery seen
}

// CorrespondenceSnapshot is the seat-scoped mesh view for whats-next.
type CorrespondenceSnapshot struct {
	PersonaID             string               `json:"persona_id,omitempty"`
	AgentID               string               `json:"agent_id,omitempty"`
	InboxUnacked          []CorrespondenceItem `json:"inbox_unacked"`
	OutboxAwaitingPeerAck []CorrespondenceItem `json:"outbox_awaiting_peer_ack"`
	RegisteredCallbacks   []PeerAckAwait       `json:"registered_callbacks,omitempty"`
	SkipReason            string               `json:"skip_reason,omitempty"`
	NextActionHint        string               `json:"next_action_hint,omitempty"`
}

const (
	HintAckThenContinue         = "ack_then_continue"
	HintContinue                = "continue"
	HintEmitStatus              = "emit_status"
	HintStandbyForbidden        = "standby_forbidden" // legacy; prefer await_peer_ack_keep_working
	HintAwaitPeerAckKeepWorking = "await_peer_ack_keep" + "_working"
)

// LoadCorrespondence builds inbox/outbox for a seat from the agent chat channel JSONL.
func LoadCorrespondence(projectRoot string, seat Seat, limit int) (CorrespondenceSnapshot, error) {
	out := CorrespondenceSnapshot{
		PersonaID:             strings.TrimSpace(seat.PersonaRef),
		AgentID:               strings.TrimSpace(seat.AgentID),
		InboxUnacked:          nil,
		OutboxAwaitingPeerAck: nil,
	}
	if out.AgentID == "" {
		out.SkipReason = "agent_id_required"
		return out, nil
	}
	if limit <= 0 {
		limit = 20
	}

	if paths.IsAgentWorktreePath(projectRoot) {
		if seated := paths.LoadBrandSettingsProjectRoot(projectRoot); seated != "" && !paths.IsAgentWorktreePath(seated) {
			projectRoot = seated
		} else {
			return out, errfmt.Errorf("refusing to load correspondence from unbonded agent worktree: %s", projectRoot)
		}
	}

	cfg, err := datacell.ReadAgentChatChannelConfig(projectRoot)
	if err != nil {
		return out, errfmt.Errorf("read agent chat channel config: %w", err)
	}
	path := datacell.EffectiveAgentChatChannelEventsJSONLPath(projectRoot, cfg)
	events, err := readJSONLEvents(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			if awaits, aerr := ListOpenPeerAckAwaits(projectRoot, out.AgentID); aerr == nil {
				out.RegisteredCallbacks = awaits
			}
			out.NextActionHint = HintContinue
			return out, nil
		}
		return out, err
	}

	peerAcked := map[string]struct{}{}   // parent event_id → valid peer_ack for that parent
	deliveryOK := map[string]struct{}{}  // parent event_id → has delivery_receipt
	myPeerAcked := map[string]struct{}{} // parent → this seat peer_acked
	byID := map[string]map[string]any{}
	for _, ev := range events {
		if eid := objects.StringField(ev, JSONFieldEventID); eid != "" {
			byID[eid] = ev
		}
	}

	for _, ev := range events {
		et := eventTypeOf(ev)
		ir := objects.StringField(ev, JSONFieldInReplyTo)
		if ir == "" {
			continue
		}
		switch et {
		case FeedEventTypePeerAck:
			aid := objects.StringField(ev, "agent_id")
			if aid == FeedKernelAgentID {
				continue
			}
			// Impersonation must not clear outbox: only an ack from the
			// addressed seat (or undirected parent) counts.
			parent := byID[ir]
			if parent != nil && !PeerAckSatisfiesParent(parent, aid) {
				continue
			}
			if parent == nil {
				// Legacy / missing parent: accept non-kernel ack (prior behavior).
			}
			peerAcked[ir] = struct{}{}
			if seatMatchesAgent(seat, aid) || seatMatchesPersona(seat, objects.StringField(ev, objects.FieldKeyPersonaRef)) {
				myPeerAcked[ir] = struct{}{}
			}
		case FeedEventTypeDeliveryReceipt:
			deliveryOK[ir] = struct{}{}
		}
	}

	var inbox, outbox []CorrespondenceItem
	for _, ev := range events {
		et := eventTypeOf(ev)
		if et == FeedEventTypeKernelAck || et == FeedEventTypePeerAck || et == FeedEventTypeDeliveryReceipt {
			continue
		}
		if et != "" && et != FeedEventTypeSteering && et != FeedEventTypeMeshStatus && et != FeedEventTypeChat && et != FeedEventTypeScanTests && et != FeedEventTypeCheckpoint && et != FeedEventTypePromote && et != FeedEventTypeWake {
			continue
		}
		eid := objects.StringField(ev, JSONFieldEventID)
		if eid == "" {
			continue // cannot correlate legacy rows without event_id
		}
		from := objects.StringField(ev, "agent_id")
		msg := objects.StringField(ev, "message")
		ts := objects.StringField(ev, "timestamp")
		to := objects.StringField(ev, JSONFieldToAgentID)
		_, hasDel := deliveryOK[eid]
		_, hasPeer := peerAcked[eid]

		item := CorrespondenceItem{
			EventID:             eid,
			Timestamp:           ts,
			Summary:             truncateSummary(msg, 160),
			Message:             msg,
			EventType:           et,
			FromAgentID:         from,
			ToAgentID:           to,
			HasDeliveryReceipt:  hasDel,
			DeliveryReceiptOnly: hasDel,
		}

		mine := seatMatchesAgent(seat, from)
		if mine {
			// Only directed collaboration events await peer_ack. Broadcast
			// mesh_status (no to_agent_id) must not fill outbox or agents hit
			// standby_forbidden after every emit-status and idle out of the loop.
			if !hasPeer && expectsPeerAck(ev, et, to) {
				outbox = append(outbox, item)
			}
			continue
		}

		// Inbox: addressed to this seat and we have not peer_acked.
		// Directed steers/chats only — status stamps are fire-and-forget.
		if et == FeedEventTypeMeshStatus && strings.TrimSpace(to) == "" {
			continue
		}
		if !addressedToSeat(seat, ev, msg) {
			continue
		}
		if _, ok := myPeerAcked[eid]; ok {
			continue
		}
		if et == FeedEventTypeMeshStatus && !expectsPeerAck(ev, et, to) {
			continue
		}
		inbox = append(inbox, item)
	}

	// Keep most recent (file is append-only; reverse then cap).
	inbox = lastN(inbox, limit)
	outbox = lastN(outbox, limit)
	out.InboxUnacked = inbox
	out.OutboxAwaitingPeerAck = outbox
	if awaits, aerr := ListOpenPeerAckAwaits(projectRoot, out.AgentID); aerr == nil {
		out.RegisteredCallbacks = awaits
	}
	out.NextActionHint = deriveHint(inbox, outbox)
	return out, nil
}

// ListInboxUnacked returns unacked inbound items for the seat.
func ListInboxUnacked(projectRoot string, seat Seat, limit int) ([]CorrespondenceItem, error) {
	snap, err := LoadCorrespondence(projectRoot, seat, limit)
	return snap.InboxUnacked, err
}

// ListOutboxAwaitingPeerAck returns outbound items still missing peer_ack.
func ListOutboxAwaitingPeerAck(projectRoot string, seat Seat, limit int) ([]CorrespondenceItem, error) {
	snap, err := LoadCorrespondence(projectRoot, seat, limit)
	return snap.OutboxAwaitingPeerAck, err
}

func deriveHint(inbox, outbox []CorrespondenceItem) string {
	if len(inbox) > 0 {
		return HintAckThenContinue
	}
	if len(outbox) > 0 {
		// Directed steer still awaiting peer_ack — do not re-blast the same ask;
		// keep working other ATKs / emit-status / poll whats-next (not full idle).
		return HintAwaitPeerAckKeepWorking
	}
	return HintContinue
}

// expectsPeerAck reports whether an outbound event should remain in
// outbox_awaiting_peer_ack until a peer_ack arrives.
//
// Collaboration contract: only directed steering/chat (explicit to_agent_id)
// require a cognitive peer receipt. Untargeted mesh_status is a broadcast
// stamp and must not block the seat with standby_forbidden.
// TRACK: [REDACTED-ID] — continuous TPM↔AGY duty cycle / MCP notify.
func expectsPeerAck(ev map[string]any, eventType, toAgentID string) bool {
	to := strings.TrimSpace(toAgentID)
	if to == "" {
		to = strings.TrimSpace(objects.StringField(ev, JSONFieldToAgentID))
	}
	switch eventType {
	case FeedEventTypeSteering, FeedEventTypeChat, FeedEventTypeScanTests, FeedEventTypeCheckpoint, FeedEventTypePromote, FeedEventTypeWake:
		return to != ""
	case FeedEventTypeMeshStatus:
		// Rare: status explicitly addressed to a peer seat still awaits ack.
		return to != ""
	default:
		return false
	}
}

func readJSONLEvents(path string) ([]map[string]any, error) {
	f, err := fileutil.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:gosec
	var events []map[string]any
	sc := bufio.NewScanner(f)
	// Large steer bodies; raise buffer.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 2*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		events = append(events, m)
	}
	return events, sc.Err()
}

func eventTypeOf(ev map[string]any) string {
	return strings.ToLower(strings.TrimSpace(objects.StringField(ev, objects.FieldKeyEventType)))
}

func seatMatchesAgent(seat Seat, agentID string) bool {
	a := strings.TrimSpace(seat.AgentID)
	b := strings.TrimSpace(agentID)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return false
}

func seatMatchesPersona(seat Seat, personaRef string) bool {
	a := strings.TrimSpace(seat.PersonaRef)
	b := strings.TrimSpace(personaRef)
	if a == "" || b == "" {
		return false
	}
	return a == b
}

func addressedToSeat(seat Seat, ev map[string]any, msg string) bool {
	to := objects.StringField(ev, JSONFieldToAgentID)
	if to != "" {
		return seatMatchesAgent(seat, to)
	}
	upper := strings.ToUpper(msg)
	hints := seat.RoleHints
	if len(hints) == 0 {
		hints = inferRoleHints(seat.AgentID)
	}
	for _, h := range hints {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		attn := "ATTN " + strings.ToUpper(h)
		if strings.Contains(upper, attn) {
			return true
		}
	}
	return false
}

func inferRoleHints(agentID string) []string {
	id := strings.TrimSpace(agentID)
	if id == "" {
		return nil
	}
	// Exact seat id only. Vendor nicknames belong on Seat.RoleHints.
	return []string{id}
}

func truncateSummary(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func lastN(items []CorrespondenceItem, n int) []CorrespondenceItem {
	if n <= 0 || len(items) <= n {
		return items
	}
	return items[len(items)-n:]
}
