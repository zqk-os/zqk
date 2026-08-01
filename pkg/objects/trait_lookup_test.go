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
