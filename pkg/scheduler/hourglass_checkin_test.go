package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/agentclaim"
)

// TestActionForExpiredTimer_checkinNeverKills is the safety property behind the cadence
// timer. The watcher's fallback for an unrecognized timer type is SIGKILL on the recorded
// pid, so if a check-in timer ever routed to the default, a claim that merely went quiet
// would have its agent process killed and its task forced to error.
func TestActionForExpiredTimer_checkinNeverKills(t *testing.T) {
	t.Parallel()
	if got := actionForExpiredTimer(agentclaim.TimerTypeCheckin); got != actionWakeOrchestrator {
		t.Fatalf("check-in routed to %v, want actionWakeOrchestrator; any other action kills or moves a task whose holder may be alive", got)
	}
}

func TestActionForExpiredTimer_routing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		timerType string
		want      expiredTimerAction
		why       string
	}{
		{"deadline", timerTypeDeadline, actionEscalateDeadline, "dispatch deadlines file a risk_blocker and defer"},
		{"sync loop", "", actionKillStuckProcess, "the empty type is the sync-loop kill timer and must keep killing"},
		{"unknown", "some_future_timer", actionKillStuckProcess, "unrecognized types fall back to kill; new types must register explicitly"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := actionForExpiredTimer(tc.timerType); got != tc.want {
				t.Fatalf("type %q routed to %v, want %v: %s", tc.timerType, got, tc.want, tc.why)
			}
		})
	}
}

// TestCheckinTimerType_isNotTheKillSentinel guards against a refactor that sets the
// constant to "" and silently turns every claim timer into a process killer.
func TestCheckinTimerType_isNotTheKillSentinel(t *testing.T) {
	t.Parallel()
	if agentclaim.TimerTypeCheckin == "" || agentclaim.TimerTypeCheckin == timerTypeDeadline {
		t.Fatalf("TimerTypeCheckin = %q; it must be a distinct non-empty type", agentclaim.TimerTypeCheckin)
	}
}
