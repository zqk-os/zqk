package whatsnext

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestPriorityPlanPackagingCue(t *testing.T) {
	t.Parallel()

	planID := "[REDACTED-ID]"
	title := "State machine cleanup"

	tests := []struct {
		name    string
		status  string
		counts  map[string]int
		wantCue bool
	}{
		{
			name:    "wrap when active and all complete",
			status:  objects.ObjectStatusActive,
			counts:  map[string]int{objects.ObjectStatusComplete: 3},
			wantCue: true,
		},
		{
			name:    "wrap when in_progress and all complete",
			status:  objects.ObjectStatusInProgress,
			counts:  map[string]int{objects.ObjectStatusComplete: 2, objects.ObjectStatusArchived: 1},
			wantCue: true,
		},
		{
			name:    "no spam with open planned children",
			status:  objects.ObjectStatusActive,
			counts:  map[string]int{objects.ObjectStatusComplete: 2, objects.ObjectStatusPlanned: 1},
			wantCue: false,
		},
		{
			name:    "no spam with in_progress children",
			status:  objects.ObjectStatusInProgress,
			counts:  map[string]int{objects.ObjectStatusInProgress: 1},
			wantCue: false,
		},
		{
			name:    "skip grooming plans",
			status:  objects.ObjectStatusGrooming,
			counts:  map[string]int{objects.ObjectStatusComplete: 3},
			wantCue: false,
		},
		{
			name:    "skip empty plan",
			status:  objects.ObjectStatusActive,
			counts:  map[string]int{},
			wantCue: false,
		},
		{
			name:    "cue after promote to complete",
			status:  objects.ObjectStatusComplete,
			counts:  map[string]int{objects.ObjectStatusComplete: 1},
			wantCue: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PriorityPlanPackagingCue(planID, title, tc.status, tc.counts)
			if tc.wantCue {
				if got == "" {
					t.Fatal("expected packaging cue, got empty")
				}
				if !strings.Contains(got, planID) {
					t.Fatalf("cue missing plan id: %q", got)
				}
				if !strings.Contains(got, title) {
					t.Fatalf("cue missing plan title: %q", got)
				}
				if !strings.Contains(got, "PRI≈PR") {
					t.Fatalf("cue missing PRI≈PR marker: %q", got)
				}
				return
			}
			if got != "" {
				t.Fatalf("expected no cue, got %q", got)
			}
		})
	}
}
