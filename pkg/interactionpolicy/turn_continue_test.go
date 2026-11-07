package interactionpolicy

import (
	"strings"
	"testing"
)

func TestCommandHintIsDispatchRemint(t *testing.T) {
	t.Parallel()
	if !CommandHintIsDispatchRemint("ORCHESTRATE_PLAN PRI-1") {
		t.Fatal("ORCHESTRATE_PLAN must remint")
	}
	if !CommandHintIsDispatchRemint("zqk feed steer --to-agent-id peer-agent-1 --await-peer-ack") {
		t.Fatal("directed steer must remint")
	}
	if !CommandHintIsDispatchRemint("zqk-tip agent swarm-init --plan-id PRI-1 --allow-chat") {
		t.Fatal("swarm-init must remint")
	}
	if !CommandHintIsDispatchRemint(hintSwarmInitSuffix) {
		t.Fatal("catalog swarm-init suffix must remint")
	}
	if CommandHintIsDispatchRemint(HintAlignRefresh()) {
		t.Fatal("align hint must not remint")
	}
}

func TestCompileTurnContinue_HourglassSuppressesSwarmInitRemint(t *testing.T) {
	t.Parallel()
	got := CompileTurnContinue(TurnContinueInput{
		GuidingEvent:          EventIdle,
		GuidingStep:           "Lead has planned BLIs. Hand off via swarm orchestration.",
		CommandHint:           HintSwarmInit("PRI-1"),
		PolicyID:              PolicyTPMProcessAdmin,
		CapLoopActive:         true,
		OutboxAwaitingPeerAck: 2,
		FillItemCommandHint:   "system kernel-integrity heal-dangling --apply",
	})
	if !got.Continue {
		t.Fatal("must continue under hourglass")
	}
	if got.Reason != reasonHourglassKeepWork {
		t.Fatalf("reason=%q", got.Reason)
	}
	if strings.Contains(got.Message, "swarm-init") && strings.Contains(got.Message, "Next:") {
		t.Fatalf("must not remint Next: swarm-init: %q", got.Message)
	}
	if !strings.Contains(got.Message, "await_peer_ack_keep_working") {
		t.Fatalf("hourglass keep-working missing: %q", got.Message)
	}
	if !strings.Contains(got.Message, "heal-dangling") {
		t.Fatalf("fill heal path missing: %q", got.Message)
	}
}

func TestCompileTurnContinue_Inbox(t *testing.T) {
	t.Parallel()
	got := CompileTurnContinue(TurnContinueInput{
		InboxUnacked: 2,
		PolicyID:     PolicyTPMProcessAdmin,
		GuidingStep:  "Ack inbox then keep working.",
		GuidingEvent: EventIdle,
	})
	if !got.Continue || got.Event != EventInboxUnacked || got.Reason != reasonInboxUnacked {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got.Message, PolicyTPMProcessAdmin) {
		t.Fatalf("missing policy: %q", got.Message)
	}
}

func TestCompileTurnContinue_IdleKeepWorking_RequiresCap(t *testing.T) {
	t.Parallel()
	silent := CompileTurnContinue(TurnContinueInput{
		InboxUnacked: 0,
		GuidingEvent: EventIdle,
		GuidingStep:  "Lead has planned BLIs.",
		PolicyID:     PolicyTPMProcessAdmin,
	})
	if silent.Continue {
		t.Fatalf("non-CAP chat must be allowed to stop, got %+v", silent)
	}
	got := CompileTurnContinue(TurnContinueInput{
		InboxUnacked:  0,
		GuidingEvent:  EventIdle,
		GuidingStep:   "Lead has planned BLIs. Hand off via swarm orchestration.",
		CommandHint:   "",
		PolicyID:      PolicyTPMProcessAdmin,
		CapLoopActive: true,
	})
	if !got.Continue {
		t.Fatalf("CAP idle with guiding step must continue, got %+v", got)
	}
	if !strings.Contains(got.Message, "planned BLIs") {
		t.Fatalf("message=%q", got.Message)
	}
}

func TestCompileTurnContinue_StripsDispatchRemintHint(t *testing.T) {
	t.Parallel()
	got := CompileTurnContinue(TurnContinueInput{
		GuidingEvent:  EventIdle,
		GuidingStep:   "Keep working process admin.",
		CommandHint:   HintOrchestratePlan("peer-agent-1", "PRI-1"),
		PolicyID:      PolicyTPMProcessAdmin,
		CapLoopActive: true,
	})
	if !got.Continue {
		t.Fatal("must continue after strip")
	}
	if strings.Contains(got.Message, "ORCHESTRATE_PLAN") {
		t.Fatalf("remint must not land in host followup: %q", got.Message)
	}
	if !strings.Contains(got.Message, capLoopPreamble) {
		t.Fatalf("CAP preamble missing: %q", got.Message)
	}
}

func TestCompileTurnContinue_CapActiveDefault(t *testing.T) {
	t.Parallel()
	got := CompileTurnContinue(TurnContinueInput{CapLoopActive: true})
	if !got.Continue || !strings.Contains(got.Message, "whats-next") {
		t.Fatalf("got %+v", got)
	}
}

func TestCompileTurnContinue_NoDrive(t *testing.T) {
	t.Parallel()
	got := CompileTurnContinue(TurnContinueInput{})
	if got.Continue || got.Reason != reasonNoDrive {
		t.Fatalf("got %+v", got)
	}
}
