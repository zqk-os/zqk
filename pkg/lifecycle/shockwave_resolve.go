package lifecycle

import (
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

func isParkMembraneStatus(to string) bool {
	switch strings.ToLower(strings.TrimSpace(to)) {
	case objects.ObjectStatusArchived, objects.ObjectStatusDeferred, objects.ObjectStatusRoadmap:
		return true
	default:
		return false
	}
}

// ResolveShockwavePolicy merges destination-status policy, then the matching
// transition (wildcard first, exact from wins), then archive defaults for park membranes.
func ResolveShockwavePolicy(lc *objects.Lifecycle, from, to string) objects.ShockwavePolicy {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	var p objects.ShockwavePolicy
	if lc != nil {
		for _, st := range lc.Statuses {
			if strings.EqualFold(strings.TrimSpace(st.Value), to) {
				p = objects.MergeShockwavePolicy(p, st.Shockwave)
				break
			}
		}
		var wildcard, exact objects.ShockwavePolicy
		var hasWildcard, hasExact bool
		for _, tr := range lc.Transitions {
			if !strings.EqualFold(strings.TrimSpace(tr.To), to) {
				continue
			}
			f := strings.ToLower(strings.TrimSpace(tr.From))
			if f == "*" {
				wildcard = tr.Shockwave
				hasWildcard = true
			}
			if f == from {
				exact = tr.Shockwave
				hasExact = true
			}
		}
		if hasWildcard {
			p = objects.MergeShockwavePolicy(p, wildcard)
		}
		if hasExact {
			p = objects.MergeShockwavePolicy(p, exact)
		}
	}
	// Empty mode on a park membrane defaults to cluster. Explicit none/shared/prune stay.
	if p.Mode == emptyValue && isParkMembraneStatus(to) {
		p = objects.MergeShockwavePolicy(objects.DefaultArchiveShockwavePolicy(), p)
		if p.Mode == emptyValue {
			p.Mode = objects.ShockwaveModeCluster
		}
	}
	if p.Mode == emptyValue || p.Mode == objects.ShockwaveModeNone {
		return p
	}
	return objects.FillShockwavePolicyDefaults(p)
}

func lineageRank(kind string) int {
	switch kind {
	case kindVision, kindWorkstream, kindRoadmap, kindStrategicPlan:
		return 0
	case kindMission:
		return 1
	case kindGoal:
		return 2
	case kindMilestone:
		return 3
	case kindPriorityPlan:
		return 4
	default:
		return 100
	}
}

func pruneAcceptsKind(seedKind, depKind string, p objects.ShockwavePolicy) bool {
	if !p.IsLineageKind(depKind) {
		return true
	}
	return lineageRank(depKind) > lineageRank(seedKind)
}
