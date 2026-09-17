package object

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/objectget"
	"github.com/lanceman/zqk/pkg/objects"
)

// applyObjectGetOverlays applies the shared "reference resolver overlay" and any
// higher-level view overlays (e.g. milestone completion/progress).
//
// Hydration policy is centralized in pkg/objectget for CLI, MCP, and hooks.
func applyObjectGetOverlays(proc *cli.Processor, obj map[string]any, viewName string, hydration objectget.LinkHydration) {
	kind, _ := obj[objects.FieldKeyKind].(string)
	plan := objectget.OverlayPlanFor(kind, viewName, hydration)

	applyReferenceResolverOverlay(proc, obj, plan.ReferenceResolver)
	if plan.ApplyMilestoneCriteriaOverlay {
		applyMilestoneCriteriaOverlay(proc, obj)
	}
	if plan.ApplyChildRollupOverlay {
		applyChildRollupOverlay(proc, obj)
	}
}

func applyObjectViewProjection(viewName string, obj map[string]any) map[string]any {
	switch viewName {
	case objectget.ViewDefault, "":
		return obj
	case objectget.ViewMilestoneCompletionReport:
		return objects.PickTopLevelFields(obj, []string{
			objects.FieldKeyID,
			objects.FieldKeyKind,
			objects.FieldKeyTitle,
			objects.FieldKeyStatus,
			objects.FieldKeyPriorityTier,
			"child_rollup",
			"milestone_complete",
			"milestone_percent_complete",
			"milestone_complete_via_criteria_refs",
			"milestone_percent_complete_via_criteria_refs",
			"resolved_" + objects.FieldKeyCriteriaRefs,
		})
	case objectget.ViewMilestoneProgressReport:
		return objects.PickTopLevelFields(obj, []string{
			objects.FieldKeyID,
			objects.FieldKeyKind,
			objects.FieldKeyTitle,
			objects.FieldKeyStatus,
			objects.FieldKeyPriorityTier,
			"child_rollup",
			"milestone_complete",
			"milestone_percent_complete",
			"milestone_percent_complete_via_criteria_refs",
			"resolved_" + objects.FieldKeyCriteriaRefs,
		})
	default:
		// Unknown view: return full object rather than surprising users by dropping fields.
		return obj
	}
}
