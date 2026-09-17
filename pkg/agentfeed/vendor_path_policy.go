package agentfeed

import (
	"strings"
	"sync/atomic"
)

// VendorPathPolicy detects when kernel contracts leak into vendor-specific
// "brain" directories. Markers are data — add/remove as the IDE/agent market shifts
// without rewriting doctor call sites.
//
// TRACK: CRIT-COMMS-007 — keep markers aligned with real vendor layout churn.
type VendorPathPolicy struct {
	// ForbiddenMarkers are path/content substrings that must not appear in
	// project-root, resolved feed paths, or lite policy payloads.
	ForbiddenMarkers []string
}

// DefaultVendorBrainMarkers are known vendor-local state trees that must not
// host ZQK kernel contracts (feeds, lite policy, mesh state).
var DefaultVendorBrainMarkers = []string{
	".gemini",
	".claude",
	".windsurf",
	".continue",
	".aider",
}

var vendorPathPolicy atomic.Pointer[VendorPathPolicy]

func init() {
	vendorPathPolicy.Store(&VendorPathPolicy{
		ForbiddenMarkers: append([]string(nil), DefaultVendorBrainMarkers...),
	})
}

// SetVendorPathPolicy replaces the process-wide vendor path policy (tests / composition).
// The stored policy is a snapshot; callers may mutate their local copy after Set.
func SetVendorPathPolicy(p VendorPathPolicy) {
	markers := p.ForbiddenMarkers
	if len(markers) == 0 {
		markers = DefaultVendorBrainMarkers
	}
	vendorPathPolicy.Store(&VendorPathPolicy{
		ForbiddenMarkers: append([]string(nil), markers...),
	})
}

// CurrentVendorPathPolicy returns a copy of the active vendor path policy.
func CurrentVendorPathPolicy() VendorPathPolicy {
	p := vendorPathPolicy.Load()
	if p == nil {
		return VendorPathPolicy{ForbiddenMarkers: append([]string(nil), DefaultVendorBrainMarkers...)}
	}
	return VendorPathPolicy{ForbiddenMarkers: append([]string(nil), p.ForbiddenMarkers...)}
}

// FindLeak reports the first forbidden marker found in s (case-sensitive path match).
func (p VendorPathPolicy) FindLeak(s string) (marker string, ok bool) {
	if strings.TrimSpace(s) == "" {
		return "", false
	}
	for _, m := range p.ForbiddenMarkers {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if strings.Contains(s, m) {
			return m, true
		}
	}
	return "", false
}

// LeakIssues returns zero or more diagnostic issue strings for the inspected texts.
func (p VendorPathPolicy) LeakIssues(projectRoot, feedPath, litePolicy string) []string {
	var issues []string
	if m, ok := p.FindLeak(projectRoot); ok {
		issues = append(issues, "vendor_path_leak: root path contains "+m)
	}
	if m, ok := p.FindLeak(feedPath); ok {
		issues = append(issues, "vendor_path_leak: resolved feed path uses vendor layout ("+m+")")
	}
	if m, ok := p.FindLeak(litePolicy); ok {
		issues = append(issues, "vendor_path_leak: lite policy contains "+m)
	}
	return issues
}
