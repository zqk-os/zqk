package objects

// PromoteTransitionTargets returns one-hop status values reachable via promote
// from current. Auto-only edges (Auto && !Manual) are excluded — those are
// system/shockwave paths, not agent promote targets. Dual edges (Manual && Auto)
// are promote-reachable and may still be shockwaved (e.g. priority_plan → complete).
// Recipe to promote-enable a former auto-only edge: LIFECYCLE_STATUS_ROLES.md
// § First-class promote. TRACK
func PromoteTransitionTargets(lifecycle *Lifecycle, current string) map[string]struct{} {
	out := make(map[string]struct{})
	if lifecycle == nil || current == "" {
		return out
	}
	for _, tr := range lifecycle.Transitions {
		if tr.To == "" {
			continue
		}
		if tr.From != current && tr.From != "*" {
			continue
		}
		if tr.Auto && !tr.Manual {
			continue
		}
		out[tr.To] = struct{}{}
	}
	return out
}

// TransitionIsAutoOnly reports system-driven edges (not promote candidates).
func TransitionIsAutoOnly(tr Transition) bool {
	return tr.Auto && !tr.Manual
}

// TransitionClearFields returns lifecycle YAML side_effects.clear names for the
// first matching from→to edge. Promote must apply these on the in-memory probe
// and persist them (storage.FieldUnset) or composed_integrity rejects hops such
// as priority_plan active→complete while active_order is still set.
// TRACK: follow-up in kernel backlog
func TransitionClearFields(lifecycle *Lifecycle, from, to string) []string {
	if lifecycle == nil || from == "" || to == "" {
		return nil
	}
	for i := range lifecycle.Transitions {
		tr := &lifecycle.Transitions[i]
		if tr.To != to {
			continue
		}
		if tr.From != from && tr.From != "*" {
			continue
		}
		if TransitionIsAutoOnly(*tr) {
			continue
		}
		out := make([]string, 0, len(tr.SideEffects))
		for _, se := range tr.SideEffects {
			if se.Clear != "" {
				out = append(out, se.Clear)
			}
		}
		return out
	}
	return nil
}
