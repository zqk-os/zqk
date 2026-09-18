package objectget

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/storage"
)

// Overlay metadata keys injected by applyReferenceResolverOverlay / object get.
const (
	overlayKeyApplied   = "reference_resolver_overlay_applied"
	overlayKeyPrefix    = "reference_resolver_overlay_"
	resolvedFieldPrefix = "resolved_"
)

// IsReferenceResolverOverlayFieldKey reports whether k is get-time hydration metadata
// (reference_resolver_overlay_* or resolved_*_ref(s)), not durable spec fields like resolved_at.
//
// TRACK: BLI-1785909672838827000-9fca84f5 — remove when: resolved sidecar is the only hydrate path
// and write paths no longer accept get-output round-trips.
func IsReferenceResolverOverlayFieldKey(k string) bool {
	if k == overlayKeyApplied || strings.HasPrefix(k, overlayKeyPrefix) {
		return true
	}
	if !strings.HasPrefix(k, resolvedFieldPrefix) {
		return false
	}
	rest := strings.TrimPrefix(k, resolvedFieldPrefix)
	return strings.HasSuffix(rest, "_ref") || strings.HasSuffix(rest, "_refs")
}

// StripReferenceResolverOverlayFields removes get-time hydration keys from obj in place.
// FieldUnset sentinels are kept so --unset-field can scrub polluted CAS instances.
// Returns the number of keys removed. Safe on nil.
func StripReferenceResolverOverlayFields(obj map[string]any) int {
	if obj == nil {
		return 0
	}
	n := 0
	for k, v := range obj {
		if !IsReferenceResolverOverlayFieldKey(k) {
			continue
		}
		if storage.IsFieldUnset(v) {
			continue
		}
		delete(obj, k)
		n++
	}
	return n
}
