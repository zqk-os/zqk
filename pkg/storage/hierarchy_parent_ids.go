package storage

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// hierarchyParentRefKeys are the child→parent membership/composition fields the
// status gateway and lifecycle occupancy fan-out walk. Singular "milestone_ref"
// remains for ATK fixtures; production BLIs store FieldKeyMilestoneRefs.
var hierarchyParentRefKeys = []string{
	"parent_ref",
	objects.FieldKeyPriorityPlanRef,
	"strategic_plan_ref",
	"milestone_ref",
	objects.FieldKeyMilestoneRefs,
	objects.FieldKeyGoalRefs,
	objects.FieldKeyBacklogItemRef,
	objects.FieldKeyEpicRef,
	objects.FieldKeyEpicRefs,
}

// HierarchyParentIDs returns unique parent object IDs for hierarchical occupancy
// shockwave (ATK→BLI→PRI/MIL→goal). It does not include persona or criteria refs.
func HierarchyParentIDs(obj map[string]any) []string {
	if obj == nil {
		return nil
	}
	seen := make(map[string]bool, len(hierarchyParentRefKeys))
	out := make([]string, 0, len(hierarchyParentRefKeys))
	for _, key := range hierarchyParentRefKeys {
		for _, id := range stringIDsFromRefField(obj[key]) {
			if id == emptyValue || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func stringIDsFromRefField(v any) []string {
	switch t := v.(type) {
	case string:
		s := strings.TrimSpace(t)
		if s == emptyValue {
			return nil
		}
		return []string{s}
	case []string:
		out := make([]string, 0, len(t))
		for _, raw := range t {
			s := strings.TrimSpace(raw)
			if s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// HierarchyBubbleToStatus is the parent hop the status gateway should apply.
// Goals have no execution_locked token; active is the executing occupancy.
func HierarchyBubbleToStatus(parentKind, parentStatus string) (to string, hop bool) {
	switch parentStatus {
	case objects.ObjectStatusInProgress, objects.ObjectStatusComplete, objects.ObjectStatusArchived:
		return emptyValue, false
	}
	if parentKind == objects.KindGoal {
		if parentStatus == objects.ObjectStatusActive {
			return emptyValue, false
		}
		if parentStatus == objects.ObjectStatusProposed {
			return objects.ObjectStatusActive, true
		}
		return emptyValue, false
	}
	switch parentStatus {
	case objects.ObjectStatusNotStarted, objects.ObjectStatusActive, objects.ObjectStatusPlanned,
		objects.ObjectStatusApproved, objects.ObjectStatusGrooming, objects.ObjectStatusBlocked:
		return objects.ObjectStatusInProgress, true
	default:
		return emptyValue, false
	}
}
