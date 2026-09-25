package scenario

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	refFieldPluralSuffix = "_refs"
	refFieldSingleSuffix = "_ref"
	fieldSemanticRef     = "reference"
	emptyValue           = ""
)

// DeferredRef indicates a ref field that should be set after create (because the target kind is created later).
type DeferredRef struct {
	Kind  string // kind that has the field
	Field string // field name (e.g. requirement_refs)
}

// refFieldNameToTargetKind infers the target kind from a reference field name.
// Convention: goal_refs -> goal, requirement_refs -> requirement, backlog_item_refs -> backlog_item.
func refFieldNameToTargetKind(fieldName string) string {
	if strings.HasSuffix(fieldName, refFieldPluralSuffix) {
		return strings.TrimSuffix(fieldName, refFieldPluralSuffix)
	}
	if strings.HasSuffix(fieldName, refFieldSingleSuffix) {
		return strings.TrimSuffix(fieldName, refFieldSingleSuffix)
	}
	return emptyValue
}

// getRefTargetKinds returns the set of kinds that this kind references (via ref fields) that are in allowedSet.
func getRefTargetKinds(idx *objects.SpecIndex, kind string, allowedSet map[string]bool) map[string]string {
	out := make(map[string]string) // target kind -> ref field name (one representative)
	if idx == nil {
		return out
	}
	summary, ok := idx.GetKindSummary(kind)
	if !ok {
		return out
	}
	for _, f := range summary.Fields {
		if !f.IsRef && f.SemanticType != fieldSemanticRef {
			continue
		}
		target := refFieldNameToTargetKind(f.Name)
		if target != emptyValue && allowedSet[target] {
			out[target] = f.Name
		}
	}
	return out
}

// CreateOrderFromSpecIndex returns a create order for the given bundle kinds by inspecting
// the spec index: kinds that are referenced by others must be created first. When the index
// is unavailable, returns a default order. Parent-owned composition (requirement.criteria_refs)
// is a DAG — criteria before requirement, no deferred reverse on criteria.
func CreateOrderFromSpecIndex(projectRoot string, bundleKinds []string) (createOrder []string, deferredRefs []DeferredRef, err error) {
	allowed := make(map[string]bool)
	for _, k := range bundleKinds {
		allowed[k] = true
	}

	idx := objects.TryLoadSpecIndexForProjectRoot(projectRoot)
	if idx == nil {
		// No index: use default order that matches current behavior (criteria before requirement, then update criteria)
		return defaultCreateOrder(bundleKinds), defaultDeferredRefs(bundleKinds), nil
	}

	// Build dependency graph: dep[kind] = set of kinds that must be created before kind (kind references them).
	deps := make(map[string]map[string]struct{})
	for _, k := range bundleKinds {
		targets := getRefTargetKinds(idx, k, allowed)
		deps[k] = make(map[string]struct{})
		for t := range targets {
			deps[k][t] = struct{}{}
		}
	}

	// Topological sort: output kinds so that for every edge (T -> K), T appears before K.
	// If there's a cycle, we break it by choosing an order and recording deferred refs.
	order, deferred := topologicalOrderWithCycles(bundleKinds, deps)
	return order, deferred, nil
}

// defaultCreateOrder returns a safe default order when spec index is not available.
func defaultCreateOrder(bundleKinds []string) []string {
	// Fixed order: criteria before requirement/milestone so parent.criteria_refs can resolve.
	preferred := []string{
		objects.KindGoal,
		objects.KindPriorityPlan,
		objects.KindCriteria,
		objects.KindMilestone,
		objects.KindRequirement,
		objects.KindDocEntry,
		objects.KindConvergenceSession,
		objects.KindTestCase,
		objects.KindBacklogItem,
	}
	seen := make(map[string]bool)
	for _, k := range preferred {
		seen[k] = true
	}
	out := make([]string, 0, len(bundleKinds))
	for _, k := range preferred {
		if inSlice(bundleKinds, k) {
			out = append(out, k)
		}
	}
	for _, k := range bundleKinds {
		if !seen[k] {
			out = append(out, k)
		}
	}
	return out
}

func defaultDeferredRefs(bundleKinds []string) []DeferredRef {
	// REQ↔CRIT is parent-owned (requirement.criteria_refs). No reverse to defer.
	return nil
}

func inSlice(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

// topologicalOrderWithCycles returns create order and deferred refs. Breaks remaining
// self-kind cycles with cycleBreakOrder. REQ↔CRIT is a DAG (parent criteria_refs only).
func topologicalOrderWithCycles(kinds []string, deps map[string]map[string]struct{}) (order []string, deferred []DeferredRef) {
	// Build reverse deps: kind -> set of kinds that depend on it (reference it).
	revDeps := make(map[string]map[string]struct{})
	for _, k := range kinds {
		revDeps[k] = make(map[string]struct{})
	}
	for k, set := range deps {
		for t := range set {
			revDeps[t][k] = struct{}{}
		}
	}

	order = make([]string, 0, len(kinds))
	remaining := make(map[string]bool)
	for _, k := range kinds {
		remaining[k] = true
	}

	// Deterministic order for remaining cycles: criteria before parents that list criteria_refs.
	cycleBreakOrder := []string{
		objects.KindGoal,
		objects.KindPriorityPlan,
		objects.KindCriteria,
		objects.KindMilestone,
		objects.KindRequirement,
		objects.KindTestCase,
		objects.KindBacklogItem,
	}

	for len(remaining) > 0 {
		// Find a kind with no remaining dependency (all deps already in order).
		var next string
		for k := range remaining {
			canCreate := true
			for dep := range deps[k] {
				if remaining[dep] {
					canCreate = false
					break
				}
			}
			if canCreate {
				next = k
				break
			}
		}
		if next != emptyValue {
			order = append(order, next)
			delete(remaining, next)
			continue
		}
		// Cycle: pick next from cycleBreakOrder so we create criteria before requirement.
		found := false
		for _, k := range cycleBreakOrder {
			if !remaining[k] {
				continue
			}
			order = append(order, k)
			delete(remaining, k)
			found = true
			break
		}
		if !found {
			// Fallback: pick any remaining kind deterministically to ensure loop termination
			var fallbackKind string
			for k := range remaining {
				if fallbackKind == "" || k < fallbackKind {
					fallbackKind = k
				}
			}
			if fallbackKind != "" {
				order = append(order, fallbackKind)
				delete(remaining, fallbackKind)
			}
		}
	}

	return order, deferred
}

// BundleKindsForTraceability returns the list of object kinds that scenario bundles can create (traceability + fixtures).
func BundleKindsForTraceability() []string {
	return []string{
		objects.KindGoal,
		objects.KindPriorityPlan,
		objects.KindMilestone,
		objects.KindCriteria,
		objects.KindRequirement,
		objects.KindTestCase,
		objects.KindBacklogItem,
	}
}
