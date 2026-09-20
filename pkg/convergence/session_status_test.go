package convergence

import "testing"

func TestSessionStatusBehaviorMatrix_OptionA(t *testing.T) {
	type row struct {
		status       string
		cap          bool
		measure      bool
		whatsNext    bool
		autoEscalate bool
	}
	cases := []row{
		{SessionStatusDraft, false, false, false, false},
		{SessionStatusActive, true, true, true, true},
		{SessionStatusPaused, true, false, true, true},
		{SessionStatusEscalated, true, false, false, false},
		{SessionStatusError, false, false, false, false},
		{SessionStatusCompleted, false, false, false, false},
		{SessionStatusAbandoned, false, false, false, false},
		{SessionStatusArchived, false, false, false, false},
		{" ESCALATED ", true, false, false, false},
	}
	for _, tc := range cases {
		if got := SessionStatusEligibleForCAP(tc.status); got != tc.cap {
			t.Errorf("%q CAP: got %v want %v", tc.status, got, tc.cap)
		}
		if got := SessionStatusPersistsMeasurement(tc.status); got != tc.measure {
			t.Errorf("%q measure: got %v want %v", tc.status, got, tc.measure)
		}
		if got := SessionStatusListedForWhatsNextMeasure(tc.status); got != tc.whatsNext {
			t.Errorf("%q whats-next: got %v want %v", tc.status, got, tc.whatsNext)
		}
		if got := SessionStatusEligibleForAutoStaleEscalate(tc.status); got != tc.autoEscalate {
			t.Errorf("%q auto-escalate: got %v want %v", tc.status, got, tc.autoEscalate)
		}
		if SessionStatusSkipsMeasurementPersist(tc.status) == tc.measure {
			t.Errorf("%q skip-measure should invert persists", tc.status)
		}
	}
}
