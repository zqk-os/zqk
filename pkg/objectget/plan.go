package objectget

import (
	"github.com/lanceman/zqk/pkg/objects"
)

// View names for object get (--view). Keep in sync with cmd/zqk/object projections.
const (
	ViewDefault                   = "default"
	ViewMilestoneCompletionReport = "milestone-completion-report"
	ViewMilestoneProgressReport   = "milestone-progress-report"
)

// OverlayPlan combines reference resolver settings with optional milestone-specific overlays.
type OverlayPlan struct {
	ReferenceResolver             ReferenceResolverOverlayConfig
	ApplyMilestoneCriteriaOverlay bool
}

// overlayPlanKindAny matches object package fallback bucket for non-milestone kinds.
const overlayPlanKindAny = "*"

var overlayPlansByKindAndView = map[string]map[string]OverlayPlan{
	objects.KindMilestone: {
		ViewDefault: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewDefault),
			ApplyMilestoneCriteriaOverlay: true,
		},
		ViewMilestoneCompletionReport: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewMilestoneCompletionReport),
			ApplyMilestoneCriteriaOverlay: true,
		},
		ViewMilestoneProgressReport: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewMilestoneProgressReport),
			ApplyMilestoneCriteriaOverlay: true,
		},
	},
	overlayPlanKindAny: {
		ViewDefault: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewDefault),
			ApplyMilestoneCriteriaOverlay: false,
		},
		ViewMilestoneCompletionReport: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewDefault),
			ApplyMilestoneCriteriaOverlay: false,
		},
		ViewMilestoneProgressReport: {
			ReferenceResolver:             ReferenceOverlayConfigForView(ViewDefault),
			ApplyMilestoneCriteriaOverlay: false,
		},
	},
}

// OverlayPlanFor returns resolver + milestone overlay flags for kind and view.
// hydration overrides reference resolver settings when not HydrationUnspecified (kind/view still
// controls milestone criteria overlay unless that changes in a future revision).
func OverlayPlanFor(kind, viewName string, hydration LinkHydration) OverlayPlan {
	if viewName == "" {
		viewName = ViewDefault
	}

	plan := lookupOverlayPlan(kind, viewName)
	if hydration != HydrationUnspecified {
		plan.ReferenceResolver = ReferenceOverlayConfigForHydration(hydration)
	}
	return plan
}

func lookupOverlayPlan(kind, viewName string) OverlayPlan {
	if byKind, ok := overlayPlansByKindAndView[kind]; ok {
		if p, ok := byKind[viewName]; ok {
			return p
		}
		if p, ok := byKind[ViewDefault]; ok {
			return p
		}
	}

	if byAny, ok := overlayPlansByKindAndView[overlayPlanKindAny]; ok {
		if p, ok := byAny[viewName]; ok {
			return p
		}
		if p, ok := byAny[ViewDefault]; ok {
			return p
		}
	}

	return OverlayPlan{
		ReferenceResolver:             ReferenceOverlayConfigForView(ViewDefault),
		ApplyMilestoneCriteriaOverlay: false,
	}
}
