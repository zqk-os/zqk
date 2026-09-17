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
	HydrationNone                      // raw CAS; no resolved_* embeds
	HydrationLazy
	HydrationDefaultExplicit
	HydrationEager
)

// ParseLinkHydration parses CLI / env style tokens (none/off/omit/raw, lazy, default, eager).
// Empty string => Unspecified (view-driven; ViewDefault is raw).
func ParseLinkHydration(s string) (LinkHydration, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "":
		return HydrationUnspecified, nil
	case "none", "off", "omit", "raw":
		return HydrationNone, nil
	case "lazy":
		return HydrationLazy, nil
	case "default":
		return HydrationDefaultExplicit, nil
	case "eager":
		return HydrationEager, nil
	default:
		return HydrationUnspecified, errfmt.Errorf("invalid link hydration %q (want none, lazy, default, or eager)", s)
	}
}

func (h LinkHydration) String() string {
	switch h {
	case HydrationNone:
		return "none"
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
	case HydrationNone:
		return ReferenceResolverOverlayConfig{Disabled: true}
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
		// Default get is raw CAS (hash seal). Opt in via --link-hydration lazy|default|eager.
		// TRACK: BLI-1785909672838827000-9fca84f5 — sidecar index replaces inline opt-in later.
		return ReferenceOverlayConfigForHydration(HydrationNone)
	default:
		return ReferenceResolverOverlayConfig{Disabled: true}
	}
}
