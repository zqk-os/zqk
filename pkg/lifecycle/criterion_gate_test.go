package lifecycle

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestCriterionStatusMeetsMilestoneGateForMilestone(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{objects.ObjectStatusValidated, true},
		{"complete", true},
		{objects.ObjectStatusCompleted, true},
		{"in_progress", false},
		{"not_started", false},
	}
	for _, tt := range tests {
		if got := CriterionStatusMeetsMilestoneGateForMilestone(tt.status); got != tt.want {
			t.Errorf("status %q: got %v want %v", tt.status, got, tt.want)
		}
	}
}

func TestStringRefsFromAny(t *testing.T) {
	if got := StringRefsFromAny([]string{"A", "B"}); len(got) != 2 || got[0] != "A" {
		t.Fatalf("[]string: %+v", got)
	}
	if got := StringRefsFromAny([]any{"X", "Y"}); len(got) != 2 || got[1] != "Y" {
		t.Fatalf("[]any: %+v", got)
	}
	if got := StringRefsFromAny(nil); len(got) != 0 {
		t.Fatalf("nil: %+v", got)
	}
	if got := StringRefsFromAny("VIS-1"); len(got) != 1 || got[0] != "VIS-1" {
		t.Fatalf("scalar string: %+v", got)
	}
	if got := StringRefsFromAny(""); len(got) != 0 {
		t.Fatalf("empty string: %+v", got)
	}
}
