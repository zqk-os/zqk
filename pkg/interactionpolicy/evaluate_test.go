package interactionpolicy

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestClassifyShell(t *testing.T) {
	t.Parallel()
	cases := []struct {
		cmd  string
		want string
	}{
		{"go test ./pkg/foo -timeout 30s", EventGoTest},
		{"zqk scheduler scan-tests --package ./pkg/foo", EventGoTest},
		{"git commit -m msg", EventGitCommit},
		{"git worktree add .zqk/worktrees/ATK-1 HEAD", EventGitWorktreeAdd},
		{"git worktree add /tmp/zqk-worktrees/repo/ATK-1 HEAD", ""},
		{"zqk agent orchestrate --plan PRI-1", EventAgentOrchestrate},
		{"zqk agent prepare-context --persona-ref PER-X ATK-1 && zqk agent orchestrate", ""},
		{"zqk agent execute ATK-1", EventAgentExecute},
		{"ls", ""},
	}
	for _, tc := range cases {
		if got := ClassifyShell(tc.cmd); got != tc.want {
			t.Errorf("ClassifyShell(%q)=%q want %q", tc.cmd, got, tc.want)
		}
	}
}

func TestEvaluate_DefaultCatalog(t *testing.T) {
	t.Parallel()
	idle := Evaluate(nil, EventIdle)
	if !idle.Matched || idle.Step == nil || idle.Step.PolicyID != PolicyTPMProcessAdmin {
		t.Fatalf("idle default=%+v", idle)
	}
	if strings.Contains(idle.Step.GuidingStep, "dispatch ORCHESTRATE_PLAN") {
		t.Fatalf("idle must not remint orch: %q", idle.Step.GuidingStep)
	}
	inbox := Evaluate(nil, EventInboxUnacked)
	if !inbox.Matched || inbox.Step == nil || inbox.Step.PolicyID != PolicyTPMProcessAdmin {
		t.Fatalf("inbox default=%+v", inbox)
	}
	if strings.Contains(inbox.Step.GuidingStep, "hourglass-steer the next BLI") {
		t.Fatalf("inbox must not hourglass park/ack theatre: %q", inbox.Step.GuidingStep)
	}
	push := Evaluate(nil, EventPushAhead)
	if !push.Matched || push.Step == nil || push.Step.PolicyID != PolicyTPMProcessAdmin {
		t.Fatalf("push-ahead default=%+v", push)
	}
	if push.Step.CommandHint != HintPushAhead {
		t.Fatalf("push hint=%q", push.Step.CommandHint)
	}
	empty := Evaluate(nil, EventShovelReadyEmpty)
	if !empty.Matched || empty.Step == nil || empty.Step.PolicyID != PolicyTPMGroomAhead {
		t.Fatalf("empty-column default=%+v", empty)
	}
	ahead := Evaluate(nil, EventStratplanAhead)
	if !ahead.Matched || ahead.Step == nil || ahead.Step.PolicyID != PolicyTPMGroomAhead {
		t.Fatalf("stratplan-ahead default=%+v", ahead)
	}
}

func TestEvaluate_PersonaFilter(t *testing.T) {
	t.Parallel()
	p := map[string]any{
		objects.FieldKeyInteractionPolicyRefs: []any{PolicyCommsRemedy},
	}
	idle := Evaluate(p, EventIdle)
	if idle.Matched {
		t.Fatalf("TPM-only persona should not get idle groom when unbound: %+v", idle)
	}
	inbox := Evaluate(p, EventInboxUnacked)
	if inbox.Matched {
		t.Fatalf("inbox PROCESS-ADMIN must be persona-bound: %+v", inbox)
	}
	push := Evaluate(p, EventPushAhead)
	if push.Matched {
		t.Fatalf("push-ahead PROCESS-ADMIN must be persona-bound: %+v", push)
	}
	comms := Evaluate(p, EventCommsFail)
	if !comms.Matched || comms.Step.PolicyID != PolicyCommsRemedy {
		t.Fatalf("comms=%+v", comms)
	}
}

func TestEvaluate_UnknownEvent(t *testing.T) {
	t.Parallel()
	got := Evaluate(nil, "not_an_event")
	if got.Matched {
		t.Fatalf("matched unknown: %+v", got)
	}
}
