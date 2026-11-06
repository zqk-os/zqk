package newcmd

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// applyPriorityPlanRelationalFlags maps --plan, --req, and --goal references into
// related_object_refs for priority_plan objects (GH-1001).
// Priority plans model relationships via related_object_refs rather than direct
// requirement_refs or goal_refs.
func applyPriorityPlanRelationalFlags(objData map[string]any, planRefs, reqRefs, goalRefs []string) {
	if objData == nil {
		return
	}
	var toAdd []string
	for _, ref := range planRefs {
		if r := strings.TrimSpace(ref); r != "" {
			toAdd = append(toAdd, r)
		}
	}
	for _, ref := range reqRefs {
		if r := strings.TrimSpace(ref); r != "" {
			toAdd = append(toAdd, r)
		}
	}
	for _, ref := range goalRefs {
		if r := strings.TrimSpace(ref); r != "" {
			toAdd = append(toAdd, r)
		}
	}
	if len(toAdd) == 0 {
		return
	}

	// Remove any stray requirement_refs or goal_refs that might have been populated previously
	delete(objData, objects.FieldKeyRequirementRefs)
	delete(objData, objects.FieldKeyGoalRefs)

	appendRefToObjData(objData, objects.FieldKeyRelatedObjectRefs, toAdd...)
}
