package interactionpolicy

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestSkipCASOverlay(t *testing.T) {
	t.Parallel()
	if !SkipCASOverlay(EventIdle) || !SkipCASOverlay(EventPushAhead) || !SkipCASOverlay(EventInboxUnacked) {
		t.Fatal("idle, push-ahead, and inbox must keep catalog text")
	}
	if SkipCASOverlay(EventStratplanAhead) {
		t.Fatal("stratplan still overlay GROOM-AHEAD")
	}
}

func TestDriveFromPolicyBody_ResponseSection(t *testing.T) {
	t.Parallel()
	body := `# TPM interaction: forward planning when planned is empty

**Persona:** TPM

**Response (prompt, do not auto-mutate locked plans):**
Do not break the seal on an in_progress/complete priority_plan. TPM: strategic planning with the stratplan team; refresh align; shape the next unlocked priority_plan. Only if the seated priority_plan is still grooming/active and list shows exploring/validated, promote those to planned.

## Not
Ignore this part.
`
	got := DriveFromPolicyBody(body)
	if !strings.Contains(got, "Do not break the seal") {
		t.Fatalf("drive=%q", got)
	}
	if strings.Contains(got, "whats-next") {
		t.Fatalf("hunger must not tell the agent to re-run whats-next: %q", got)
	}
	if strings.Contains(got, "Ignore this") {
		t.Fatalf("leaked next heading: %q", got)
	}
}

func TestDriveFromPolicyBody_StopsAtParentField(t *testing.T) {
	t.Parallel()
	body := `**Response (prompt):**
If align-latest.json is fresh, do not re-run align; shape the next unlocked priority_plan still in grooming.

**Parent:** POL-AGENT-INTERACTION-POLICY-001
`
	got := DriveFromPolicyBody(body)
	if !strings.Contains(got, "do not re-run align") {
		t.Fatalf("drive=%q", got)
	}
	if strings.Contains(got, "Parent") || strings.Contains(got, "POL-AGENT-INTERACTION-POLICY-001") {
		t.Fatalf("leaked Parent field: %q", got)
	}
}

func TestDriveFromPolicyBody_FirstProse(t *testing.T) {
	t.Parallel()
	got := DriveFromPolicyBody("# Title\n\nFirst gland paragraph.\n\nSecond.\n")
	if got != "First gland paragraph." {
		t.Fatalf("got %q", got)
	}
}

func TestOverlayFromPolicy(t *testing.T) {
	t.Parallel()
	s := Step{PolicyID: PolicyTPMGroomAhead, GuidingStep: "catalog reflex"}
	ok := OverlayFromPolicy(&s, map[string]any{
		objects.FieldKeyBody: "**Response:**\nGroom the lead PRI before orchestrate.\n",
	})
	if !ok || s.GuidingStep != "Groom the lead PRI before orchestrate." {
		t.Fatalf("ok=%v step=%+v", ok, s)
	}
	if OverlayFromPolicy(&s, map[string]any{objects.FieldKeyBody: ""}) {
		t.Fatal("empty body should not overlay")
	}
}

func TestClipRunes(t *testing.T) {
	t.Parallel()
	got := DriveFromPolicyBody(strings.Repeat("x", maxDriveRunes+8))
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("want clipped drive, got len=%d %q", len(got), got)
	}
}
