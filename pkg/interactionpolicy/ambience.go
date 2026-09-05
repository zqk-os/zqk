package interactionpolicy

import "github.com/lanceman/zqk/pkg/objects"

// Ambience is compiled micro-signals from whats-next (not a shell command).
// TRACK: REDACTED
type Ambience struct {
	Planned        int
	InProgress     int
	Exploring      int
	Validated      int
	PlanStatus     string
	InboxUnacked   int
	OutboxAwaiting int
	// BranchAhead is local commits not on the tracked upstream (0 if unknown).
	BranchAhead int
}

func planExecutionLocked(s Ambience) bool {
	if s.InProgress > 0 {
		return true
	}
	switch s.PlanStatus {
	case objects.ObjectStatusInProgress, objects.ObjectStatusComplete:
		return true
	default:
		return false
	}
}

// ClassifyAmbience picks one event from compiled signals.
// Preemption (POL-AGENT-INTERACTION-POLICY-001): inbox > push-ahead >
// planned execution (idle) > kernel fill (compiled onto idle, not a
// separate event) > groom-ahead > align. Never silence.
// TRACK: REDACTED
//
// Inbox unacked is the TPM swarm gland: MCP notify does not start a Cursor
// turn, so hunger must compile a followup_message or the seat goes idle while
// peers wait on hourglass.
//
// Push-ahead is landing duty already written on PROCESS-ADMIN (push the seated
// plan trunk). It is a classifier rung, not a new subsystem.
//
// planned==0 is not automatically "groom this priority_plan." An execution-locked
// lead (in_progress items or plan status) is stratplan_ahead: forward planning
// and alignment, never restuff/seal-break. shovel_ready_empty is only unsealed
// intake (exploring/validated present, no in-flight work). Align is the last
// rung of that event (CompileStratplanCommandHint), not a separate gland.
//
// Outbox awaiting peer_ack with planned>0 is keep-working idle, not a hole.
func ClassifyAmbience(s Ambience) string {
	if s.InboxUnacked > 0 {
		return EventInboxUnacked
	}
	if s.BranchAhead > 0 {
		return EventPushAhead
	}
	if s.Planned > 0 {
		return EventIdle
	}
	if planExecutionLocked(s) || s.Exploring+s.Validated == 0 {
		return EventStratplanAhead
	}
	return EventShovelReadyEmpty
}
