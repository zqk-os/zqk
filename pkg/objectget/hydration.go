package objectget

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
)

// LinkHydration selects reference-overlay depth/caps independent of milestone report views.
// HydrationUnspecified keeps kind+view-driven behavior (see OverlayPlan).
type LinkHydration int

const (
	HydrationUnspecified LinkHydration = iota
	HydrationLazy
	HydrationDefaultExplicit
	HydrationEager
)

// ParseLinkHydration parses CLI / env style tokens (lazy, default, eager). Empty string => Unspecified.
func ParseLinkHydration(s string) (LinkHydration, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "":
		return HydrationUnspecified, nil
	case "lazy":
		return HydrationLazy, nil
	case "default":
		return HydrationDefaultExplicit, nil
	case "eager":
		return HydrationEager, nil
	default:
		return HydrationUnspecified, errfmt.Errorf("invalid link hydration %q (want lazy, default, or eager)", s)
	}
}

func (h LinkHydration) String() string {
	switch h {
	case HydrationLazy:
		return "lazy"
	case HydrationDefaultExplicit:
		return "default"
	case HydrationEager:
		return "eager"
	default:
		return ""
	}
}

// ReferenceOverlayConfigForHydration returns overlay settings for an explicit hydration mode.
// Used when callers override view defaults (CLI --link-hydration, MCP, hooks).
func ReferenceOverlayConfigForHydration(h LinkHydration) ReferenceResolverOverlayConfig {
	switch h {
	case HydrationLazy:
		// One hop: resolve *_ref(s) on the root object only; no recursion into referenced payloads.
		return ReferenceResolverOverlayConfig{
			MaxDepth:              1,
			MaxTotalUniqueIDs:     DefaultReferenceResolverMaxTotalUniqueIDs,
			MaxTotalResolvedEntry: DefaultReferenceResolverMaxTotalResolvedEntry,
		}
	case HydrationDefaultExplicit:
		return ReferenceResolverOverlayConfig{}
	case HydrationEager:
		return ReferenceResolverOverlayConfig{
			MaxDepth:              4,
			MaxTotalUniqueIDs:     600,
			MaxTotalResolvedEntry: 4000,
		}
	default:
		return ReferenceResolverOverlayConfig{}
	}
}

// ReferenceOverlayConfigForView maps legacy view names to resolver settings (milestone report slices).
func ReferenceOverlayConfigForView(viewName string) ReferenceResolverOverlayConfig {
	switch viewName {
	case ViewMilestoneCompletionReport, ViewMilestoneProgressReport:
		return ReferenceResolverOverlayConfig{
			ResolveFieldKeys: []string{objects.FieldKeyCriteriaRefs},
			MaxDepth:         1,
		}
	case ViewDefault, "":
		return ReferenceResolverOverlayConfig{}
	default:
		return ReferenceResolverOverlayConfig{}
	}
}
