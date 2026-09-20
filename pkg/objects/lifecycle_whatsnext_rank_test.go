package objects

import "testing"

func TestPlanWhatsNextCandidateStatuses_IncludesGrooming(t *testing.T) {
	t.Parallel()
	got := PlanWhatsNextCandidateStatuses()
	want := map[string]bool{
		ObjectStatusInProgress:   true,
		ObjectStatusPaused:       true,
		ObjectStatusActive:       true,
		ObjectStatusGrooming:     true,
		ObjectStatusPrioritizing: true,
	}
	if len(got) != len(want) {
		t.Fatalf("len=%d want %d: %v", len(got), len(want), got)
	}
	for _, st := range got {
		if !want[st] {
			t.Fatalf("unexpected status %q in %v", st, got)
		}
	}
}

func TestPlanWhatsNextStatusBonus_Roles(t *testing.T) {
	if got := PlanWhatsNextStatusBonus(KindPriorityPlan, "in_progress"); got != whatsNextBonusExecutionLocked {
		t.Fatalf("in_progress: got %d want %d", got, whatsNextBonusExecutionLocked)
	}
	if got := PlanWhatsNextStatusBonus(KindPriorityPlan, "active"); got != whatsNextBonusShovelReady {
		t.Fatalf("active: got %d want %d", got, whatsNextBonusShovelReady)
	}
	if got := PlanWhatsNextStatusBonus(KindPriorityPlan, "paused"); got != whatsNextBonusHalted {
		t.Fatalf("paused: got %d want %d", got, whatsNextBonusHalted)
	}
	if got := PlanWhatsNextStatusBonus(KindPriorityPlan, "prioritizing"); got != whatsNextBonusPrioritizing {
		t.Fatalf("prioritizing: got %d want %d", got, whatsNextBonusPrioritizing)
	}
	if got := PlanWhatsNextStatusBonus(KindPriorityPlan, "grooming"); got != 0 {
		t.Fatalf("grooming: got %d want 0", got)
	}
}

func TestPlanHasWrittenIdentity_PrefersInheritedTitle(t *testing.T) {
	t.Parallel()
	if PlanHasWrittenIdentity(nil) {
		t.Fatal("nil must fail")
	}
	if PlanHasWrittenIdentity(map[string]any{}) {
		t.Fatal("empty must fail")
	}
	if !PlanHasWrittenIdentity(map[string]any{FieldKeyTitle: "CEF Round 23 — package layout"}) {
		t.Fatal("inherited title must pass")
	}
	if !PlanHasWrittenIdentity(map[string]any{FieldKeyDescription: "intake notes"}) {
		t.Fatal("specialized description must pass")
	}
}

func TestPlanHasWorkstreamLane_PluralOrSingular(t *testing.T) {
	t.Parallel()
	if PlanHasWorkstreamLane(map[string]any{FieldKeyWorkstreamRefs: []string{}}) {
		t.Fatal("empty plural must fail")
	}
	if !PlanHasWorkstreamLane(map[string]any{FieldKeyWorkstreamRefs: []string{"WS-CEF-ARCHITECTURE"}}) {
		t.Fatal("plural must pass")
	}
	if !PlanHasWorkstreamLane(map[string]any{FieldKeyWorkstreamRef: "WS-CEF-ARCHITECTURE"}) {
		t.Fatal("singular alias must pass")
	}
}

func TestPlanActiveOrderPenalty_ExecutionLockedZero(t *testing.T) {
	if got := PlanActiveOrderPenalty(KindPriorityPlan, map[string]any{FieldKeyStatus: ObjectStatusInProgress}); got != 0 {
		t.Fatalf("in_progress nil order: got %d", got)
	}
	if got := PlanActiveOrderPenalty(KindPriorityPlan, map[string]any{
		FieldKeyStatus:      ObjectStatusActive,
		FieldKeyActiveOrder: 1,
	}); got != 1 {
		t.Fatalf("active@1: got %d", got)
	}
}

func TestBacklogCountsAsOpenWork(t *testing.T) {
	if !BacklogCountsAsOpenWork("planned") || !BacklogCountsAsOpenWork("exploring") {
		t.Fatal("planned/exploring should count as open")
	}
	if BacklogCountsAsOpenWork("complete") || BacklogCountsAsOpenWork("error") {
		t.Fatal("complete/error must not count as open")
	}
}

func TestBacklogCountsAsExecutionFuel(t *testing.T) {
	t.Parallel()
	if !BacklogCountsAsExecutionFuel("planned") || !BacklogCountsAsExecutionFuel("in_progress") {
		t.Fatal("planned/in_progress must be execution fuel")
	}
	if BacklogCountsAsExecutionFuel("deferred") || BacklogCountsAsExecutionFuel("roadmap") || BacklogCountsAsExecutionFuel("validated") {
		t.Fatal("parked realign must not be execution fuel")
	}
}

func TestPlanSeatedPersonaBonus(t *testing.T) {
	t.Parallel()
	pids := []string{"PER-ORCH-ALPHA"}
	exclusive := map[string]any{FieldKeyPersonaRefs: []string{"PER-ORCH-ALPHA"}}
	shared := map[string]any{FieldKeyPersonaRefs: []string{"PER-ORCH-ALPHA", "PER-ORCH-BETA"}}
	if got := PlanSeatedPersonaBonus(nil, exclusive); got != 0 {
		t.Fatalf("unfiltered bonus=%d", got)
	}
	if got := PlanSeatedPersonaBonus(pids, exclusive); got != whatsNextExclusivePersona {
		t.Fatalf("exclusive=%d", got)
	}
	if got := PlanSeatedPersonaBonus(pids, shared); got != -whatsNextSharedPersonaPenalty {
		t.Fatalf("shared=%d", got)
	}
}
