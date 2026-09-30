package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/convergence"
)

func TestCvsStatusEligibleForCAP_MatchesConvergenceMatrix(t *testing.T) {
	for _, st := range []string{
		convergence.SessionStatusActive,
		convergence.SessionStatusPaused,
		convergence.SessionStatusEscalated,
		convergence.SessionStatusCompleted,
		convergence.SessionStatusDraft,
	} {
		if got, want := cvsStatusEligibleForCAP(st), convergence.SessionStatusEligibleForCAP(st); got != want {
			t.Errorf("%q: wrapper %v matrix %v", st, got, want)
		}
	}
}
