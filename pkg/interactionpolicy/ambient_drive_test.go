package interactionpolicy

import (
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAppendStratplanAmbient(t *testing.T) {
	t.Parallel()
	got := AppendStratplanAmbient("Do not break the seal.", StratplanAmbient{
		LeadID:          "PRI-LEAD",
		LeadTitle:       "Draft-plane ghost",
		LeadStatus:      objects.ObjectStatusInProgress,
		Counts:          map[string]int{"planned": 0, "in_progress": 1, "complete": 0},
		AlignOK:         true,
		AlignFresh:      true,
		AlignAge:        "12m",
		AlignScore:      92.9,
		GoalGaps:        62,
		DraftPlaneTotal: 5,
		NextPlans:       []string{"PRI-NEXT (grooming)"},
	})
	for _, part := range []string{
		"Do not break the seal.",
		"Ambient:",
		"PRI-LEAD",
		"in_progress",
		"planned=0",
		"align score=92.9",
		"fresh",
		"age=12m",
		"goal_gaps=62",
		"draft_plane=5",
		"PRI-NEXT",
	} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
}

func TestAppendStratplanAmbient_MissingAlign(t *testing.T) {
	t.Parallel()
	got := AppendStratplanAmbient("Forward.", StratplanAmbient{LeadID: "PRI-X", LeadStatus: objects.ObjectStatusActive})
	if !strings.Contains(got, "align cache missing") || !strings.Contains(got, "PRI-X") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "next_priority_plans none") {
		t.Fatalf("empty sibling list must be explicit, got %q", got)
	}
}

func TestCompileStratplanCommandHint_FreshAlignPointsAtNextColumn(t *testing.T) {
	t.Parallel()
	got := CompileStratplanCommandHint(StratplanAmbient{
		AlignOK:    true,
		AlignFresh: true,
		NextPlans:  []string{"PRI-CEF-R9-MEASURE-001 (grooming)"},
	})
	want := HintObjectGet("PRI-CEF-R9-MEASURE-001")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestHintOrchestratePlan(t *testing.T) {
	t.Parallel()
	got := HintOrchestratePlan("antigravity-1", "PRI-CEF-R20-BRANCH-PROVENANCE-001")
	if !strings.Contains(got, "ORCHESTRATE_PLAN PRI-CEF-R20-BRANCH-PROVENANCE-001") {
		t.Fatalf("got %q", got)
	}
	if !strings.Contains(got, "--to-agent-id antigravity-1") || !strings.Contains(got, "--await-peer-ack") {
		t.Fatalf("hourglass flags missing: %q", got)
	}
	if HintOrchestratePlan("", "") != HintHourglass() {
		t.Fatal("empty plan falls back to hourglass protocol")
	}
}

func TestCompileStratplanCommandHint_ShapedNextHourglassesExecutingLead(t *testing.T) {
	t.Parallel()
	got := CompileStratplanCommandHint(StratplanAmbient{
		AlignOK:         true,
		AlignFresh:      true,
		LeadStatus:      objects.ObjectStatusInProgress,
		Counts:          map[string]int{"in_progress": 1},
		DraftPlaneTotal: 2,
		NextPlans:       []string{"PRI-CEF-R9-MEASURE-001 (grooming,shaped)"},
	})
	if got != HintHourglass() {
		t.Fatalf("shaped next + executing lead must not remint get/draft, got %q", got)
	}
}

func TestCompileStratplanCommandHint_MissingOrStaleAlign(t *testing.T) {
	t.Parallel()
	if got := CompileStratplanCommandHint(StratplanAmbient{}); got != HintAlignRefresh() {
		t.Fatalf("missing: %q", got)
	}
	if got := CompileStratplanCommandHint(StratplanAmbient{AlignOK: true, AlignFresh: false}); got != HintAlignRefresh() {
		t.Fatalf("stale: %q", got)
	}
}

func TestCompileStratplanCommandHint_FreshAlignDraftPlane(t *testing.T) {
	t.Parallel()
	got := CompileStratplanCommandHint(StratplanAmbient{
		AlignOK:         true,
		AlignFresh:      true,
		DraftPlaneTotal: 4,
	})
	if got != HintDraftClassify() {
		t.Fatalf("got %q want %q", got, HintDraftClassify())
	}
}

func TestFinalizeAlignFreshness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 20, 7, 0, 0, 0, time.UTC)
	a := StratplanAmbient{
		AlignOK:         true,
		AlignMeasuredAt: now.Add(-12 * time.Minute).Format(time.RFC3339),
	}
	FinalizeAlignFreshness(&a, now)
	if !a.AlignFresh || a.AlignAge != "12m" {
		t.Fatalf("fresh: %+v", a)
	}
	a.AlignMeasuredAt = now.Add(-2 * time.Hour).Format(time.RFC3339)
	FinalizeAlignFreshness(&a, now)
	if a.AlignFresh || a.AlignAge != "2h" {
		t.Fatalf("stale: %+v", a)
	}
}

func TestRankNextPlanLabels_GroomingFirst(t *testing.T) {
	t.Parallel()
	got := RankNextPlanLabels("PRI-LEAD", []AmbientPlanRef{
		{ID: "PRI-LEAD", Status: objects.ObjectStatusInProgress},
		{ID: "PRI-ACTIVE", Status: objects.ObjectStatusActive},
		{ID: "PRI-NEXT", Status: objects.ObjectStatusGrooming},
	})
	if len(got) < 1 || !strings.HasPrefix(got[0], "PRI-NEXT") {
		t.Fatalf("grooming should rank first: %v", got)
	}
}

func TestRankNextPlanLabels_ShapedAnnotation(t *testing.T) {
	t.Parallel()
	got := RankNextPlanLabels("PRI-LEAD", []AmbientPlanRef{
		{ID: "PRI-NEXT", Status: objects.ObjectStatusGrooming, Shaped: true},
	})
	if len(got) != 1 || got[0] != "PRI-NEXT (grooming,shaped)" {
		t.Fatalf("got %v", got)
	}
}

func TestHintSwarmInit(t *testing.T) {
	t.Parallel()
	got := HintSwarmInit("PRI-PUBLIC-LAUNCH-READINESS-100")
	if !strings.Contains(got, "swarm-init") || !strings.Contains(got, "PRI-PUBLIC-LAUNCH-READINESS-100") || !strings.Contains(got, "--allow-chat") {
		t.Fatalf("unexpected swarm-init hint: %q", got)
	}
	gotEmpty := HintSwarmInit("")
	if !strings.Contains(gotEmpty, "swarm-init") || !strings.Contains(gotEmpty, "--allow-chat") {
		t.Fatalf("unexpected fallback swarm-init hint: %q", gotEmpty)
	}
}

