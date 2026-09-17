package storage

import "testing"

func TestBackfillWorkEnvelopeCompletedAt_NilStore(t *testing.T) {
	t.Parallel()
	got := BackfillWorkEnvelopeCompletedAt(t.Context(), nil, nil, true, 0)
	if !got.DryRun {
		t.Fatal("expected dry-run")
	}
	if len(got.Errors) == 0 {
		t.Fatal("expected storage-is-nil error")
	}
}

func TestEffortAwareKindNamesIncludesTimesheetKinds(t *testing.T) {
	t.Parallel()
	names := effortAwareKindNames()
	want := map[string]bool{
		"backlog_item":   true,
		"milestone":      true,
		"technical_debt": true,
		"agent_task":     true,
	}
	got := map[string]bool{}
	for _, k := range names {
		got[k] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing effort_aware kind %s in %#v", k, names)
		}
	}
	if got["policy"] || got["goal"] || got["priority_plan"] {
		t.Fatalf("non-timesheet kinds leaked into effort_aware set: %#v", names)
	}
}
