package logging

import (
	"bytes"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestFluentWarnChainsFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, WarnLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	Fluent(logger).Warn("fluent_test_warn").String("k", "v").Int("n", 1).Log()
	out := buf.String()
	if !strings.Contains(out, "fluent_test_warn") || !strings.Contains(out, "k=v") {
		t.Fatalf("unexpected log output: %q", out)
	}
}

func TestFluentCommonFieldHelpers(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))
	Fluent(logger).Info("fluent_common_fields").
		PlanID("PRI-1").
		TaskID("ATK-1").
		AgentID("seat-1").
		FeedID("AGF-1").
		PersonaRef("PER-DEFAULT-OPERATOR").
		Role("operator").
		Status("approved").
		ParentID("BLI-1").
		Handler("cap").
		Stage("grooming").
		Addr("127.0.0.1:8443").
		Script("scripts/x.sh").
		PRNumber(1309).
		Log()
	out := buf.String()
	for _, want := range []string{
		"plan_id=PRI-1", "task_id=ATK-1", "agent_id=seat-1", "feed_id=AGF-1",
		"persona_ref=PER-DEFAULT-OPERATOR", "role=operator", "status=approved",
		"parent_id=BLI-1", "handler=cap", "stage=grooming", "addr=127.0.0.1:8443",
		"script=scripts/x.sh", "pr_number=1309",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	// Field constructors share wire keys
	f := PlanIDField("PRI-2")
	if f.Key != "plan_id" || f.Value != "PRI-2" {
		t.Fatalf("PlanIDField=%+v", f)
	}
}

func TestFluentUnlevelledPanics(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(&buf, InfoLevel, NewTextFormatter(pkgctx.NewSystemContext()))

	// Intentionally missing a level - per TDE-F-CQ-005, do not crash daemons; fallback to Info with log_warning field
	e := &fluentEntry{
		logger: logger,
		msg:    "unlevelled message",
	}
	e.Log()

	out := buf.String()
	if !strings.Contains(out, "unlevelled message") {
		t.Fatalf("expected log output to contain message, got: %s", out)
	}
	if !strings.Contains(out, "unlevelled_entry_defaulted_to_info") {
		t.Fatalf("expected log output to contain unlevelled warning field, got: %s", out)
	}
}
