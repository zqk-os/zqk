package objects

import (
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
)

const (
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityMedium   = "medium"
	PriorityLow      = "low"

	PriorityTierP0 = "P0"
	PriorityTierP1 = "P1"
	PriorityTierP2 = "P2"
	PriorityTierP3 = "P3"
)

// PriorityToTier maps a priority string (critical, high, medium, low) to its priority tier (P0, P1, P2, P3).
// Also recognizes p0..p3 if passed as priority.
func PriorityToTier(priority string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case PriorityCritical, "p0":
		return PriorityTierP0, true
	case PriorityHigh, "p1":
		return PriorityTierP1, true
	case PriorityMedium, "p2":
		return PriorityTierP2, true
	case PriorityLow, "p3":
		return PriorityTierP3, true
	default:
		return "", false
	}
}

// TierToPriority maps a priority tier (P0, P1, P2, P3) to its priority level (critical, high, medium, low).
// Also recognizes priority strings if passed as tier.
func TierToPriority(tier string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case PriorityTierP0, "CRITICAL":
		return PriorityCritical, true
	case PriorityTierP1, "HIGH":
		return PriorityHigh, true
	case PriorityTierP2, "MEDIUM":
		return PriorityMedium, true
	case PriorityTierP3, "LOW":
		return PriorityLow, true
	default:
		return "", false
	}
}

// IsLegitimatePriority reports whether priority is a recognized priority level (critical, high, medium, low).
func IsLegitimatePriority(priority string) bool {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// IsLegitimatePriorityTier reports whether tier is a recognized priority tier (P0, P1, P2, P3).
func IsLegitimatePriorityTier(tier string) bool {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case PriorityTierP0, PriorityTierP1, PriorityTierP2, PriorityTierP3:
		return true
	default:
		return false
	}
}

// CanonicalPriority returns the canonical lowercase priority string.
func CanonicalPriority(priority string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case PriorityCritical, "p0":
		return PriorityCritical, true
	case PriorityHigh, "p1":
		return PriorityHigh, true
	case PriorityMedium, "p2":
		return PriorityMedium, true
	case PriorityLow, "p3":
		return PriorityLow, true
	default:
		return "", false
	}
}

// CanonicalPriorityTier returns the canonical uppercase priority tier string (P0, P1, P2, P3).
func CanonicalPriorityTier(tier string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case PriorityTierP0, "CRITICAL":
		return PriorityTierP0, true
	case PriorityTierP1, "HIGH":
		return PriorityTierP1, true
	case PriorityTierP2, "MEDIUM":
		return PriorityTierP2, true
	case PriorityTierP3, "LOW":
		return PriorityTierP3, true
	default:
		return "", false
	}
}

// CoerceBacklogItemPriorityAndTier pairs priority and priority_tier on backlog items.
// If one is provided, it sets the other.
// If both are provided, it validates they are legitimate values and normalizes casing.
func CoerceBacklogItemPriorityAndTier(obj map[string]any) {
	if obj == nil {
		return
	}

	rawPri, hasPri := obj[FieldKeyPriority]
	rawTier, hasTier := obj[FieldKeyPriorityTier]

	priStr := ""
	if hasPri && rawPri != nil {
		if s, ok := rawPri.(string); ok {
			priStr = strings.TrimSpace(s)
		}
	}
	tierStr := ""
	if hasTier && rawTier != nil {
		if s, ok := rawTier.(string); ok {
			tierStr = strings.TrimSpace(s)
		}
	}

	if priStr == "" && tierStr == "" {
		return
	}

	// Case 1: priority provided, priority_tier missing/empty
	if priStr != "" && tierStr == "" {
		canonPri, okPri := CanonicalPriority(priStr)
		tier, okTier := PriorityToTier(priStr)
		if okPri && okTier {
			obj[FieldKeyPriority] = canonPri
			obj[FieldKeyPriorityTier] = tier
		}
		return
	}

	// Case 2: priority_tier provided, priority missing/empty
	if tierStr != "" && priStr == "" {
		canonTier, okTier := CanonicalPriorityTier(tierStr)
		pri, okPri := TierToPriority(tierStr)
		if okTier && okPri {
			obj[FieldKeyPriority] = pri
			obj[FieldKeyPriorityTier] = canonTier
		}
		return
	}

	// Case 3: both provided - normalize casing if legitimate
	if canonPri, ok := CanonicalPriority(priStr); ok && IsLegitimatePriority(canonPri) {
		obj[FieldKeyPriority] = canonPri
	}
	if canonTier, ok := CanonicalPriorityTier(tierStr); ok && IsLegitimatePriorityTier(canonTier) {
		obj[FieldKeyPriorityTier] = canonTier
	}
}

// ValidatePriorityAndTier validates that priority and priority_tier are legitimate values if present.
func ValidatePriorityAndTier(obj map[string]any) error {
	if obj == nil {
		return nil
	}
	rawPri, hasPri := obj[FieldKeyPriority]
	rawTier, hasTier := obj[FieldKeyPriorityTier]

	if hasPri && rawPri != nil {
		if s, ok := rawPri.(string); ok && strings.TrimSpace(s) != "" {
			if !IsLegitimatePriority(s) {
				return errfmt.Errorf("invalid priority %q: must be one of critical, high, medium, low", s)
			}
		}
	}
	if hasTier && rawTier != nil {
		if s, ok := rawTier.(string); ok && strings.TrimSpace(s) != "" {
			if !IsLegitimatePriorityTier(s) {
				return errfmt.Errorf("invalid priority_tier %q: must be one of P0, P1, P2, P3", s)
			}
		}
	}
	return nil
}
