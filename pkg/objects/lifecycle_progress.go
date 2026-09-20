package objects

import (
	"sort"
	"strconv"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// NextProgressLifecycleStatus returns the next valid lifecycle transition that
// advances the primary progress path. It never chooses parking, archive,
// failure, or system-managed statuses; callers must use explicit lifecycle
// operations for those transitions.
func NextProgressLifecycleStatus(kind, currentStatus string) (string, error) {
	if kind == "" {
		return "", errfmt.Errorf("cannot derive auto-status: object is missing kind")
	}
	if currentStatus == "" {
		return "", errfmt.Errorf("cannot derive auto-status: object is missing status")
	}

	loader := GetGlobalLifecycleLoader()
	lifecycle, err := loader.LoadLifecycle(kind)
	if err != nil {
		return "", errfmt.Newf("cannot derive auto-status for kind %q", kind).Wrap(err)
	}
	canonicalCurrent, err := loader.NormalizeStatusForKind(kind, currentStatus)
	if err != nil {
		canonicalCurrent = currentStatus
	}

	currentIdx := -1
	var currentMeta Status
	for i, status := range lifecycle.Statuses {
		if status.Value == canonicalCurrent {
			currentIdx = i
			currentMeta = status
			break
		}
	}
	if currentIdx < 0 {
		return "", errfmt.Errorf(
			"cannot derive auto-status: current status %q not in lifecycle for kind %q",
			canonicalCurrent,
			kind,
		)
	}

	currentPercent := LifecycleProgressPercent(canonicalCurrent, lifecycle.PercentComplete)
	allowRecovery := currentMeta.System || currentMeta.Role == LifecycleRoleHalted
	type rankedStatus struct {
		value         string
		percent       float64
		index         int
		explicit      bool
		recoveryReady bool
	}
	candidates := make([]rankedStatus, 0, len(lifecycle.Statuses))
	for i, candidate := range lifecycle.Statuses {
		if candidate.Value == canonicalCurrent || IsNonProgressLifecycleStatus(candidate.Value, candidate) {
			continue
		}
		valid, transitionErr := loader.IsValidTransition(kind, canonicalCurrent, candidate.Value)
		if transitionErr != nil || !valid {
			continue
		}
		percent := LifecycleProgressPercent(candidate.Value, lifecycle.PercentComplete)
		if !allowRecovery && percent <= currentPercent {
			continue
		}
		candidates = append(candidates, rankedStatus{
			value:         candidate.Value,
			percent:       percent,
			index:         i,
			explicit:      hasExplicitLifecycleTransition(lifecycle, canonicalCurrent, candidate.Value),
			recoveryReady: candidate.Role == LifecycleRoleShovelReady || candidate.Role == LifecycleRoleExecutionLocked,
		})
	}
	if allowRecovery {
		hasReadyCandidate := false
		for _, candidate := range candidates {
			if candidate.recoveryReady {
				hasReadyCandidate = true
				break
			}
		}
		if hasReadyCandidate {
			ready := candidates[:0]
			for _, candidate := range candidates {
				if candidate.recoveryReady {
					ready = append(ready, candidate)
				}
			}
			candidates = ready
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].explicit != candidates[j].explicit {
			return candidates[i].explicit
		}
		if candidates[i].percent == candidates[j].percent {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].percent < candidates[j].percent
	})
	if len(candidates) > 0 {
		return candidates[0].value, nil
	}

	return "", errfmt.Errorf(
		"no progress transition from %q for kind %q; --auto-status never selects archive, parking, failure, or system statuses; use object park or demote for non-progress transitions",
		canonicalCurrent,
		kind,
	)
}

func hasExplicitLifecycleTransition(lifecycle *Lifecycle, from, to string) bool {
	for _, transition := range lifecycle.Transitions {
		if transition.From == from && transition.To == to {
			return true
		}
	}
	return false
}

// LifecycleProgressPercent returns the configured status progress used to
// order lifecycle candidates. Dynamic "calculated" statuses occupy the
// post-setup, pre-completion position.
func LifecycleProgressPercent(status string, config PercentCompleteConfig) float64 {
	value, ok := config.DefaultByStatus[status]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	case string:
		text := strings.TrimSpace(strings.TrimSuffix(typed, "%"))
		if parsed, err := strconv.ParseFloat(text, 64); err == nil {
			return parsed
		}
		if strings.EqualFold(text, "calculated") {
			return 75
		}
	}
	return 0
}

// IsNonProgressLifecycleStatus reports whether automatic forward movement must
// skip a status. Successful terminal states remain eligible so auto-status and
// promote can complete work when completion preconditions are satisfied.
func IsNonProgressLifecycleStatus(candidate string, status Status) bool {
	if status.System || status.Archive {
		return true
	}
	if status.Terminal && !IsSuccessLifecycleTerminal(candidate) {
		return true
	}
	switch candidate {
	case ObjectStatusRejected,
		ObjectStatusCancelled,
		ObjectStatusError,
		ObjectStatusRoadmap,
		ObjectStatusDeferred,
		ObjectStatusBlocked,
		ObjectStatusPaused:
		return true
	default:
		return false
	}
}

// IsSuccessLifecycleTerminal reports whether candidate represents successful
// lifecycle completion rather than archival, cancellation, or failure.
func IsSuccessLifecycleTerminal(candidate string) bool {
	switch candidate {
	case ObjectStatusComplete,
		ObjectStatusCompleted,
		ObjectStatusImplemented,
		ObjectStatusSuccess,
		ObjectStatusResolved: // TRACK: TDE-1784981879484045000-4d1fba7b — technical_debt verifying→resolved remains a success path.
		return true
	default:
		return false
	}
}
