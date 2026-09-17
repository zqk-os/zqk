package objects

import "testing"

func TestKindHasTrait_AutoStatusTransitionable(t *testing.T) {
	t.Parallel()

	has, err := KindHasTrait("criteria", "auto_status_transitionable")
	if err != nil {
		t.Fatalf("KindHasTrait(criteria, auto_status_transitionable) error: %v", err)
	}
	if !has {
		t.Fatalf("expected criteria to be auto_status_transitionable")
	}
}

func TestKindHasTrait_ExcludedAutoStatusTransitionable(t *testing.T) {
	t.Parallel()

	has, err := KindHasTrait("scheduler_job", "auto_status_transitionable")
	if err != nil {
		t.Fatalf("KindHasTrait(scheduler_job, auto_status_transitionable) error: %v", err)
	}
	if has {
		t.Fatalf("expected scheduler_job to exclude auto_status_transitionable")
	}
}

func TestKindHasTrait_WorkEnvelopeFacets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind           string
		completable    bool
		effort         bool
		satisfiable    bool
		occupiable     bool
		statusReactive bool
		openCountable  bool
	}{
		{kind: "backlog_item", completable: true, effort: true},
		{kind: "milestone", completable: true, effort: true, statusReactive: true, openCountable: true},
		{kind: "technical_debt", completable: true, effort: true},
		{kind: "agent_task", completable: true, effort: true, occupiable: true},
		{kind: "requirement", completable: true, effort: false, statusReactive: true},
		{kind: "goal", completable: true, effort: false, statusReactive: true},
		{kind: "roadmap", completable: true, effort: false},
		{kind: "workstream", completable: true, effort: false},
		{kind: "priority_plan", completable: true, effort: false, statusReactive: true, openCountable: true},
		{kind: "strategic_plan", completable: true, effort: false},
		{kind: "test_case", completable: true, effort: false, openCountable: true},
		{kind: "convergence_session", completable: true, effort: false},
		{kind: "criteria", satisfiable: true, statusReactive: true},
		{kind: "glossary_term"},
		{kind: "policy"},
		{kind: "mission"},
		{kind: "vision"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			gotC, err := KindHasTrait(tc.kind, "completable")
			if err != nil {
				t.Fatalf("KindHasTrait(%s, completable): %v", tc.kind, err)
			}
			if gotC != tc.completable {
				t.Fatalf("completable=%v want %v", gotC, tc.completable)
			}
			gotE, err := KindHasTrait(tc.kind, "effort_aware")
			if err != nil {
				t.Fatalf("KindHasTrait(%s, effort_aware): %v", tc.kind, err)
			}
			if gotE != tc.effort {
				t.Fatalf("effort_aware=%v want %v", gotE, tc.effort)
			}
			gotS, err := KindHasTrait(tc.kind, "satisfiable")
			if err != nil {
				t.Fatalf("KindHasTrait(%s, satisfiable): %v", tc.kind, err)
			}
			if gotS != tc.satisfiable {
				t.Fatalf("satisfiable=%v want %v", gotS, tc.satisfiable)
			}
			gotOcc, err := KindHasTrait(tc.kind, "occupiable")
			if err != nil {
				t.Fatalf("KindHasTrait(%s, occupiable): %v", tc.kind, err)
			}
			if gotOcc != tc.occupiable {
				t.Fatalf("occupiable=%v want %v", gotOcc, tc.occupiable)
			}
			gotSR, err := KindHasTrait(tc.kind, TraitStatusReactive)
			if err != nil {
				t.Fatalf("KindHasTrait(%s, status_reactive): %v", tc.kind, err)
			}
			if gotSR != tc.statusReactive {
				t.Fatalf("status_reactive=%v want %v", gotSR, tc.statusReactive)
			}
			gotOC, err := KindHasTrait(tc.kind, TraitOpenCountable)
			if err != nil {
				t.Fatalf("KindHasTrait(%s, open_countable): %v", tc.kind, err)
			}
			if gotOC != tc.openCountable {
				t.Fatalf("open_countable=%v want %v", gotOC, tc.openCountable)
			}
		})
	}
}
