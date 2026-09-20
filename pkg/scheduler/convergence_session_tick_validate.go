package scheduler

import (
	"regexp"
	"sync"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// Allowed convergence_session object id shape for scheduler-driven ticks (env CONVERGENCE_SESSION_ID).
// Rejects path-like or cross-kind ids before storage reads.
var convergenceSessionTickIDPattern = regexp.MustCompile(`^CVS-[0-9]{1,24}-[0-9a-f]{8,128}$`)

var convergenceTickSessionLocks sync.Map // session id -> *sync.Mutex

func validateConvergenceSessionTickTargetID(id string) error {
	if id == "" {
		return errfmt.Errorf("convergence_session_tick: empty session id after trim")
	}
	if len(id) > 160 {
		return errfmt.Errorf("convergence_session_tick: session id exceeds max length")
	}
	if !convergenceSessionTickIDPattern.MatchString(id) {
		return errfmt.Errorf("convergence_session_tick: invalid session id format (expect CVS-* object id)")
	}
	return nil
}

// acquireConvergenceSessionTickLock serializes overlapping convergence_session_tick runs for the same CVS
// when multiple scheduler workers or duplicated jobs could otherwise interleave updates.
func acquireConvergenceSessionTickLock(sessionID string) (unlock func()) {
	v, isLoaded := convergenceTickSessionLocks.LoadOrStore(sessionID, &sync.Mutex{})
	_ = isLoaded // Acknowledged
	mu := v.(*sync.Mutex)
	mu.Lock()
	return func() { mu.Unlock() }
}
