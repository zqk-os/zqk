package object

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// lifecycleStatusByValue returns the status metadata for value, or a zero Status
// when the lifecycle does not define it.
func lifecycleStatusByValue(lifecycle *objects.Lifecycle, value string) objects.Status {
	if lifecycle == nil {
		return objects.Status{}
	}
	for _, s := range lifecycle.Statuses {
		if s.Value == value {
			return s
		}
	}
	return objects.Status{}
}

// isNonProgressLifecycleProbeCandidate reports whether promote/demote should
// skip probing this status. True for system, archive, and non-success terminal
// statuses, and for lateral holding / failure statuses (blocked, paused,
// deferred, roadmap, rejected, cancelled, error) that are not forward or
// backward progress along the primary lifecycle path.
//
// Success terminals (complete / completed / implemented / success) MUST remain
// probeable: otherwise `zqk object promote` can never leave in_progress even
// when complete preconditions are satisfied.
func isNonProgressLifecycleProbeCandidate(candidate string, meta objects.Status) bool {
	return objects.IsNonProgressLifecycleStatus(candidate, meta)
}

func isSuccessLifecycleTerminal(candidate string) bool {
	return objects.IsSuccessLifecycleTerminal(candidate)
}

// promoteAllowsArchiveHop reports complete (success-terminal) → archived as a
// forward promote hop. Archive stays non-progress from mid-lifecycle statuses
// (* → archived is still park-eligible for lateral repair).
func promoteAllowsArchiveHop(current, candidate string) bool {
	return isSuccessLifecycleTerminal(current) &&
		strings.EqualFold(strings.TrimSpace(candidate), objects.ObjectStatusArchived)
}

func getPercentComplete(status string, config objects.PercentCompleteConfig) float64 {
	return objects.LifecycleProgressPercent(status, config)
}
