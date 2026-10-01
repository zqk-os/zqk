package object

import (
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/rollup"
)

// applyChildRollupOverlay applies kind-agnostic child rollup summaries (counts, percent complete,
// estimated and actual effort statistics) to container objects (e.g. milestone, priority_plan).
func applyChildRollupOverlay(proc *cli.Processor, obj map[string]any) {
	if proc == nil || obj == nil {
		return
	}

	kind, _ := obj[objects.FieldKeyKind].(string)
	id, _ := obj[objects.FieldKeyID].(string)
	if id == "" {
		return
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	sp := proc.Storage()
	if sp == nil {
		return
	}

	children, err := rollup.FetchChildrenForParent(ctx, secCtx, sp, kind, id)
	if err != nil {
		return
	}

	summary := rollup.Calculate(children)
	obj["child_rollup"] = summary.ToMap()

	// Direct convenience fields for milestones:
	if kind == objects.KindMilestone {
		isComplete := summary.TotalCount > 0 && summary.CompletedCount == summary.TotalCount
		obj["milestone_complete"] = isComplete
		obj["milestone_percent_complete"] = summary.PercentComplete

		// Keep compatibility fields populated for reports/views
		if _, exists := obj["milestone_percent_complete_via_criteria_refs"]; !exists {
			obj["milestone_complete_via_criteria_refs"] = isComplete
			obj["milestone_percent_complete_via_criteria_refs"] = summary.PercentComplete
		}
	}

	// Direct convenience fields for priority plans:
	if kind == objects.KindPriorityPlan {
		isComplete := summary.TotalCount > 0 && summary.CompletedCount == summary.TotalCount
		obj["priority_plan_complete"] = isComplete
		obj["priority_plan_percent_complete"] = summary.PercentComplete
	}
}
