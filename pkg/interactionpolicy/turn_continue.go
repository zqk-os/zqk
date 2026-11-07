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
	InboxUnacked  int
	GuidingEvent  string
	GuidingStep   string
	CommandHint   string
	PolicyID      string
	CapLoopActive bool
	SteeringNudge string
}

const (
	reasonInboxUnacked    = "inbox_unacked"
	reasonCapKeepWorking  = "cap_keep_working"
	reasonGuidingKeepWork = "guiding_keep_working"
	reasonNoDrive         = "no_drive"
	reasonRemintHintStrip = "remint_hint_stripped"
	defaultInboxContinue  = "Inbox has unacked swarm mail. Ack as this seat, then keep working (do not end the turn while peers await)."
	defaultCapContinue    = "CAP loop active. Query workflow whats-next and advance without idling."
	capLoopPreamble       = "CONTINUOUS AUTONOMOUS PROGRESSION (CAP LOOP): "
)

// CommandHintIsDispatchRemint reports hints that belong on seat-worker/scheduler
// dispatch, not host chat remint (ORCHESTRATE_PLAN / directed feed steer).
func CommandHintIsDispatchRemint(hint string) bool {
	h := strings.TrimSpace(hint)
	if h == "" {
		return false
	}
	return strings.Contains(h, "ORCHESTRATE_PLAN") ||
		strings.Contains(h, "feed steer --to-agent-id") ||
		strings.Contains(h, HintOrchestratePrefix)
}

// CompileTurnContinue decides whether a seat host must auto-continue the turn.
// Ping-pong invariant (POL-AGENT-INTERACTION-POLICY-001): never silence when
// hunger exists. Idle with planned work / CAP active still continues; remint
// dispatch hints are stripped so chat is not the scheduler.
func CompileTurnContinue(in TurnContinueInput) TurnContinue {
	ev := strings.TrimSpace(in.GuidingEvent)
	step := strings.TrimSpace(in.GuidingStep)
	hint := strings.TrimSpace(in.CommandHint)
	pol := strings.TrimSpace(in.PolicyID)
	stripped := false
	if CommandHintIsDispatchRemint(hint) {
		hint = ""
		stripped = true
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
	key := fmt.Sprintf("%s|%s|%s", pol, ev, hint)
	reason := reasonGuidingKeepWork
	if in.CapLoopActive {
		reason = reasonCapKeepWorking
	}
	if stripped {
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
