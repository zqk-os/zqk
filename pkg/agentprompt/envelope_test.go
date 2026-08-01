package agentprompt

import (
	"strings"
	"testing"
)

func TestEnvelopeFromAttentionMode_RoundTrip(t *testing.T) {
	t.Parallel()
	env, err := EnvelopeFromAttentionMode(AttentionTestNonDirective)
	if err != nil {
		t.Fatal(err)
	}
	line, err := FormatHTMLCommentLine(env)
	if err != nil {
		t.Fatal(err)
	}
	got, rest, err := ParseLeadingEnvelope(line + "# x\n")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected envelope")
	}
	if got.AttentionMode != AttentionTestNonDirective {
		t.Fatalf("attention_mode: %q", got.AttentionMode)
	}
	if got.QueuePolicy != QueuePolicyIgnoreAsOperationalDirective {
		t.Fatalf("queue_policy: %q", got.QueuePolicy)
	}
	if !strings.HasPrefix(rest, "# x") {
		t.Fatalf("remainder: %q", rest)
	}
}

func TestParseLeadingEnvelope_Absent(t *testing.T) {
	t.Parallel()
	md := "# Title\n"
	env, rest, err := ParseLeadingEnvelope(md)
	if err != nil {
		t.Fatal(err)
	}
	if env != nil {
		t.Fatalf("expected nil envelope, got %+v", env)
	}
	if rest != md {
		t.Fatalf("remainder should be unchanged")
	}
}

func TestParseLeadingEnvelope_SchemaMismatch(t *testing.T) {
	t.Parallel()
	bad := "<!-- zqk:agent_prompt {\"schema\":\"other\",\"attention_mode\":\"x\"} -->\n"
	_, _, err := ParseLeadingEnvelope(bad)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvelopeHelpers(t *testing.T) {
	t.Parallel()
	e, _ := EnvelopeFromAttentionMode(AttentionTestNonDirective)
	if !e.IgnoreAsOperationalDirective() || e.InterruptPlumbingDryRun() {
		t.Fatalf("test_non_directive helpers")
	}
	e2, _ := EnvelopeFromAttentionMode(AttentionInterruptPlumbing)
	if !e2.InterruptPlumbingDryRun() || e2.IgnoreAsOperationalDirective() {
		t.Fatalf("interrupt helpers: ignore=%v interrupt=%v", e2.IgnoreAsOperationalDirective(), e2.InterruptPlumbingDryRun())
	}
}
