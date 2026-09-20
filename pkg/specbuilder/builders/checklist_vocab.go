package builders

import (
	"slices"
	"strings"
)

// Checklist YAML and hand-written builders historically mixed a trailing period on short
// atoms (e.g. "yes" vs "yes."). We normalize by stripping at most one final '.' after
// trim, then accepting the value only when it matches a known atom. Arbitrary prose
// (multiple words, or unknown text) is left unchanged so terminal periods are kept.

var (
	checklistAtomsObservability = []string{"yes"}
	checklistAtomsSecurity      = []string{"non-sensitive"}
	checklistAtomsLifecycle     = []string{"mutable", "immutable"}
)

// normalizeChecklistAtom trims space, drops a single optional trailing period, and returns
// the canonical atom when the result is in atoms; otherwise returns s unchanged.
func normalizeChecklistAtom(s string, atoms []string) string {
	t := strings.TrimSpace(s)
	base := strings.TrimSuffix(t, ".")
	if slices.Contains(atoms, base) {
		return base
	}
	return s
}

// NormalizeChecklistObservability returns canonical wording for checklist observability.
func NormalizeChecklistObservability(s string) string {
	return normalizeChecklistAtom(s, checklistAtomsObservability)
}

// NormalizeChecklistSecurity returns canonical wording for checklist security.
func NormalizeChecklistSecurity(s string) string {
	return normalizeChecklistAtom(s, checklistAtomsSecurity)
}

// NormalizeChecklistLifecycle returns canonical wording for short lifecycle atoms; longer
// checklist prose is unchanged.
func NormalizeChecklistLifecycle(s string) string {
	return normalizeChecklistAtom(s, checklistAtomsLifecycle)
}
