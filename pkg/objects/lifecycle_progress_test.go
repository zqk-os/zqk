package objects

import (
	"strings"
	"testing"
)

func TestNextProgressLifecycleStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string
		current string
		want    string
	}{
		{
			name:    "backlog validated skips parking and archive",
			kind:    KindBacklogItem,
			current: ObjectStatusValidated,
			want:    ObjectStatusPlanned,
		},
		{
			name:    "criteria advances primary path",
			kind:    KindCriteria,
			current: ObjectStatusAwaitingVerification,
			want:    ObjectStatusInProgress,
		},
		{
			name:    "error recovers to earlier declared progress status",
			kind:    KindBacklogItem,
			current: ObjectStatusError,
			want:    ObjectStatusPlanned,
		},
		{
			name:    "active plan advances to execution lock instead of grooming",
			kind:    KindPriorityPlan,
			current: ObjectStatusActive,
			want:    ObjectStatusInProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NextProgressLifecycleStatus(tt.kind, tt.current)
			if err != nil {
				t.Fatalf("NextProgressLifecycleStatus(%q, %q): %v", tt.kind, tt.current, err)
			}
			if got != tt.want {
				t.Fatalf("NextProgressLifecycleStatus(%q, %q) = %q, want %q", tt.kind, tt.current, got, tt.want)
			}
		})
	}
}

func TestNextProgressLifecycleStatus_RefusesNonProgressOnlyEdges(t *testing.T) {
	t.Parallel()

	got, err := NextProgressLifecycleStatus(KindDecision, ObjectStatusActive)
	if err == nil {
		t.Fatalf("NextProgressLifecycleStatus(active decision) = %q, want error", got)
	}
	if !strings.Contains(err.Error(), "never selects archive, parking, failure, or system statuses") {
		t.Fatalf("expected safe-action guidance, got: %v", err)
	}
}
