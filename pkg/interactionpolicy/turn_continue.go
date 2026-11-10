package interactionpolicy

import (
	"fmt"
	"strings"
)

// TurnContinue is the vendor-agnostic "seat turn ended → keep going?" decision.
// Host adapters (Cursor stop hook, Claude block, etc.) only specialize delivery.
type TurnContinue struct {
	Continue bool   `json:"continue"`
	Message  string `json:"message,omitempty"`
	Event    string `json:"event,omitempty"`
	Key      string `json:"key,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// TurnContinueInput is compiled ambient + guiding hunger for one seat stop.
type TurnContinueInput struct {
	InboxUnacked          int
	OutboxAwaitingPeerAck int
	GuidingEvent          string
	GuidingStep           string
	CommandHint           string
	PolicyID              string
	CapLoopActive         bool
	SteeringNudge         string
	FillItemCommandHint   string
}

const (
	reasonInboxUnacked       = "inbox_unacked"
	reasonCapKeepWorking     = "cap_keep_working"
	reasonGuidingKeepWork    = "guiding_keep_working"
	reasonNoDrive            = "no_drive"
	reasonRemintHintStrip    = "remint_hint_stripped"
	reasonHourglassKeepWork  = "hourglass_keep_working"
	defaultInboxContinue     = "Inbox has unacked swarm mail. Ack as this seat, then keep working (do not end the turn while peers await)."
	defaultCapContinue       = "CAP loop active. Query workflow whats-next and advance without idling."
	defaultHourglassContinue = "Hourglass open (outbox awaiting peer_ack). Do NOT remint agent swarm-init / ORCHESTRATE_PLAN. await_peer_ack_keep_working: advance fill_item / local TPM work; ring peers only via feed steer|wake --await-peer-ack."
	// TRACK: BLI-CAP-IDLE-SUSPEND-001 — remint strip must also scrub GuidingStep swarm-init prose
	// or CAP refuses remint, burns Cursor stop loop_limit, then sits idle until a human intervenes.
	defaultRemintStrippedContinue = "Seat-worker/scheduler owns ORCHESTRATE_PLAN and swarm-init — do NOT remint from chat. Query workflow whats-next; advance fill_item / absorb peer lands / process-admin; keep CAP moving."
	capLoopPreamble               = "CONTINUOUS AUTONOMOUS PROGRESSION (CAP LOOP): "
)

// CommandHintIsDispatchRemint reports hints that belong on seat-worker/scheduler
// dispatch, not host chat remint (ORCHESTRATE_PLAN / swarm-init / directed steer).
func CommandHintIsDispatchRemint(hint string) bool {
	h := strings.TrimSpace(hint)
	if h == "" {
		return false
	}
	low := strings.ToLower(h)
	return strings.Contains(h, "ORCHESTRATE_PLAN") ||
		strings.Contains(h, "feed steer --to-agent-id") ||
		strings.Contains(h, HintOrchestratePrefix) ||
		strings.Contains(low, "swarm-init") ||
		strings.Contains(low, hintSwarmInitSuffix) ||
		strings.Contains(low, HintSwarmInitText)
}

// CompileTurnContinue decides whether a seat host must auto-continue the turn.
// Ping-pong invariant (POL-AGENT-INTERACTION-POLICY-001): never silence when
// hunger exists. Idle with planned work / CAP active still continues; remint
// dispatch hints are stripped so chat is not the scheduler. Open peer-ack
// hourglass suppresses swarm-init / ORCHESTRATE remint (await_peer_ack_keep_working).
func CompileTurnContinue(in TurnContinueInput) TurnContinue {
	ev := strings.TrimSpace(in.GuidingEvent)
	step := strings.TrimSpace(in.GuidingStep)
	hint := strings.TrimSpace(in.CommandHint)
	pol := strings.TrimSpace(in.PolicyID)
	fill := strings.TrimSpace(in.FillItemCommandHint)
	stripped := false
	if CommandHintIsDispatchRemint(hint) {
		hint = ""
		stripped = true
		step = scrubDispatchRemintProse(step)
	}
	hourglass := in.OutboxAwaitingPeerAck > 0
	if hourglass {
		// Never re-issue dispatch remints while peers owe acks on directed steers.
		hint = ""
		stripped = true
		step = defaultHourglassContinue
		if fill != "" {
			step = step + " fill_item: " + fill + " (prefer kernel-integrity heal-dangling --apply over stale autofix dangling wording)."
		}
	}

	// Host auto-continue is for CAP / swarm seats only. Ordinary IDE chats must
	// be allowed to stop (marker/plan absent and empty inbox → no drive).
	if in.InboxUnacked == 0 && !in.CapLoopActive {
		return TurnContinue{Continue: false, Reason: reasonNoDrive, Event: ev}
	}

	if in.InboxUnacked > 0 {
		msg := step
		if msg == "" {
			msg = defaultInboxContinue
		}
		msg = joinPolicy(pol, msg)
		if hint != "" && !strings.Contains(msg, hint) {
			msg = msg + " Next: " + hint
		}
		if n := strings.TrimSpace(in.SteeringNudge); n != "" {
			msg = msg + n
		}
		if in.CapLoopActive {
			msg = capLoopPreamble + msg
		}
		key := fmt.Sprintf("%s|%s|%s", pol, EventInboxUnacked, hint)
		return TurnContinue{
			Continue: true,
			Message:  msg,
			Event:    EventInboxUnacked,
			Key:      key,
			Reason:   reasonInboxUnacked,
		}
	}

	if step == "" {
		step = defaultCapContinue
		if ev == "" {
			ev = EventIdle
		}
	}

	msg := joinPolicy(pol, step)
	if hint != "" && !strings.Contains(msg, hint) {
		msg = msg + " Next: " + hint
	}
	if n := strings.TrimSpace(in.SteeringNudge); n != "" {
		msg = msg + n
	}
	if in.CapLoopActive {
		msg = capLoopPreamble + msg
	}
	if ev == "" {
		ev = EventIdle
	}
	key := fmt.Sprintf("%s|%s|%s|hg=%d", pol, ev, hint, in.OutboxAwaitingPeerAck)
	reason := reasonGuidingKeepWork
	if in.CapLoopActive {
		reason = reasonCapKeepWorking
	}
	if hourglass {
		reason = reasonHourglassKeepWork
	} else if stripped {
		reason = reasonRemintHintStrip
	}
	return TurnContinue{
		Continue: true,
		Message:  msg,
		Event:    ev,
		Key:      key,
		Reason:   reason,
	}
}

func joinPolicy(policyID, step string) string {
	if policyID == "" {
		return step
	}
	if strings.HasPrefix(step, policyID) {
		return step
	}
	return policyID + ": " + step
}

// scrubDispatchRemintProse replaces catalog idle text that still tells chat to
// hand off via swarm-init/orchestrate after CommandHint was stripped. Leaving
// that prose in the followup is what makes CAP refuse → stop → loop_limit out.
func scrubDispatchRemintProse(step string) string {
	s := strings.TrimSpace(step)
	if s == "" {
		return defaultRemintStrippedContinue
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "swarm-init") ||
		strings.Contains(low, "swarm init") ||
		strings.Contains(low, "orchestrate") ||
		strings.Contains(low, "anti-piecemeal") ||
		strings.Contains(low, "hand off batch") {
		return defaultRemintStrippedContinue
	}
	return s
}
