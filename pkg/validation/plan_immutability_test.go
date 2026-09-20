package validation

import "testing"

func TestPlanStatusAllowsBacklogInProgress(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{"in_progress", true},
		{"ACTIVE", true},
		{"grooming", false},
		{"complete", false},
		{"paused", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := PlanStatusAllowsBacklogInProgress(tc.status); got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.status, got, tc.want)
		}
	}
}
