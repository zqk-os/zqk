package convergence

import "strings"

// Session status behavior matrix (DEC Option A — park-but-CAP-bound for escalated).
// TRACK: REDACTED — keep callers on these helpers, not ad-hoc switches.
//
//	status      | draft_plane | CAP bind | measure persist | whats-next measure | auto-stale→escalated
//	------------|------------|----------|-----------------|--------------------|---------------------
//	draft       | yes        | no       | no              | no                 | no
//	active      | no         | yes      | yes             | yes                | yes
//	paused      | no         | yes      | no              | yes                | yes
//	escalated   | no         | yes      | no              | no                 | n/a
//	error       | no         | no       | no              | no                 | no
//	completed   | no         | no       | no              | no                 | no
//	abandoned   | no         | no       | no              | no                 | no
//	archived    | no         | no       | no              | no                 | no

const (
	SessionStatusDraft     = "draft"
	SessionStatusActive    = "active"
	SessionStatusPaused    = "paused"
	SessionStatusEscalated = "escalated"
	SessionStatusError     = "error"
	SessionStatusCompleted = "completed"
	SessionStatusAbandoned = "abandoned"
	SessionStatusArchived  = "archived"
)

func normalizeSessionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

// SessionStatusEligibleForCAP reports whether CAP may bind / advance against this session.
// Option A: escalated stays CAP-eligible so overnight parents are not unbound by escalate noise.
func SessionStatusEligibleForCAP(status string) bool {
	switch normalizeSessionStatus(status) {
	case SessionStatusActive, SessionStatusPaused, SessionStatusEscalated:
		return true
	default:
		return false
	}
}

// SessionStatusPersistsMeasurement is true when convergence measure/tick may write
// measurement fields onto the session object.
func SessionStatusPersistsMeasurement(status string) bool {
	return normalizeSessionStatus(status) == SessionStatusActive
}

// SessionStatusListedForWhatsNextMeasure is true when whats-next may auto-select
// this session as the measure target (explicit --session-id may still override).
func SessionStatusListedForWhatsNextMeasure(status string) bool {
	switch normalizeSessionStatus(status) {
	case SessionStatusActive, SessionStatusPaused:
		return true
	default:
		return false
	}
}

// SessionStatusEligibleForAutoStaleEscalate is true when convergence_engine may
// move a stale session to escalated after the inactivity timeout.
func SessionStatusEligibleForAutoStaleEscalate(status string) bool {
	switch normalizeSessionStatus(status) {
	case SessionStatusActive, SessionStatusPaused:
		return true
	default:
		return false
	}
}

// SessionStatusSkipsMeasurementPersist is the inverse of PersistsMeasurement for
// tick handlers that also treat lifecycle-terminal statuses as skip.
func SessionStatusSkipsMeasurementPersist(status string) bool {
	return !SessionStatusPersistsMeasurement(status)
}
