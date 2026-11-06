package validation

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// NormalizePriority normalizes and coerces priority representations based on object kind.
// For requirements: maps word aliases (critical, high, medium, low) to canonical p-tier (p0, p1, p2, p3).
// For backlog_items: maps p-tier aliases (p0..p3) to canonical words (critical, high, medium, low)
// and synchronizes priority_tier (P0..P3).
func NormalizePriority(kind string, obj map[string]any) {
	objects.NormalizePriority(kind, obj)
}

// CoercePriority is an alias for NormalizePriority.
func CoercePriority(kind string, obj map[string]any) {
	objects.CoercePriority(kind, obj)
}

// ValidatePriority validates that a priority string is recognized across both p-tier and word alias spectrums.
func ValidatePriority(kind string, priority string) error {
	if kind != "" {
		return objects.ValidatePriorityForKind(kind, priority)
	}
	return objects.ValidatePriority(priority)
}

// ValidatePriorityAndTier validates that priority and priority_tier in an object map are legitimate values.
func ValidatePriorityAndTier(obj map[string]any) error {
	return objects.ValidatePriorityAndTier(obj)
}
