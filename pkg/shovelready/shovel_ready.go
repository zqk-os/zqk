package shovelready

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/objects"
)

// CriteriaID is the canonical CRI-SHOVEL-READY criteria object.
const CriteriaID = "CRIT-SHOVEL-READY"

// Precondition is the exact lifecycle precondition string. checkPrecondition must
// recognize this token; English theater ("Owner confirms…") is fail-open.
const Precondition = "CRI-SHOVEL-READY"

// RequiresPromoteGate is true for transitions INTO planned/in_progress except
// in_progress→planned (pause/demote recovery must not demand DoR fields).
func RequiresPromoteGate(from, to string) bool {
	from = strings.ToLower(strings.TrimSpace(from))
	to = strings.ToLower(strings.TrimSpace(to))
	if to != objects.ObjectStatusPlanned && to != objects.ObjectStatusInProgress {
		return false
	}
	if from == objects.ObjectStatusInProgress && to == objects.ObjectStatusPlanned {
		return false
	}
	return true
}

// Result is the CRI-SHOVEL-READY gate outcome for one backlog item.
type Result struct {
	Ready   bool
	Missing []string
}

// Evaluate checks MMORCH CRI-SHOVEL-READY against a backlog_item map.
// AUTO-/synthetic grooming rows (id prefix AUTO-BLI-) are treated as ready so TPM
// empty-plan recovery is not blocked by the dispatch gate.
func Evaluate(bli map[string]any) Result {
	if bli == nil {
		return Result{Ready: false, Missing: []string{"object"}}
	}
	id, _ := bli[objects.FieldKeyID].(string)
	if strings.HasPrefix(strings.TrimSpace(id), "AUTO-BLI-") {
		return Result{Ready: true}
	}

	var missing []string
	if !hasNonEmptyRefList(bli, objects.FieldKeyRequirementRefs) &&
		!hasNonEmptyRefList(bli, objects.FieldKeyTechnicalSpecRefs) {
		missing = append(missing, "requirement_refs|technical_spec_refs")
	}
	if !hasNonEmptyRefList(bli, objects.FieldKeyCriteriaRefs) {
		missing = append(missing, objects.FieldKeyCriteriaRefs)
	}
	if !hasEstimatedScope(bli) {
		missing = append(missing, objects.FieldKeyEstimatedEffort+"|scope_signal")
	}
	if !hasLaneAssignment(bli) {
		missing = append(missing, "persona|stakeholders")
	}
	if hasUnresolvedBlocker(bli) {
		missing = append(missing, "unresolved_blocker")
	}
	return Result{Ready: len(missing) == 0, Missing: missing}
}

// IsReady is the boolean form of Evaluate.
func IsReady(bli map[string]any) bool {
	return Evaluate(bli).Ready
}

func hasEstimatedScope(bli map[string]any) bool {
	if s, ok := bli[objects.FieldKeyEstimatedEffort].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	for _, key := range []string{objects.FieldKeyPriorityTier, objects.FieldKeyPriority, "scope", "size"} {
		if s, ok := bli[key].(string); ok {
			norm := strings.ToLower(strings.TrimSpace(s))
			switch norm {
			case "small", "medium", "large", "s", "m", "l",
				"p0", "p1", "p2", "p3", "high", "low", "critical":
				return true
			}
			if norm != "" && (strings.HasPrefix(norm, "p0") || strings.HasPrefix(norm, "p1") ||
				strings.HasPrefix(norm, "p2") || strings.HasPrefix(norm, "p3")) {
				return true
			}
		}
	}
	return false
}

func hasLaneAssignment(bli map[string]any) bool {
	if hasAnyStringRef(bli, objects.FieldKeyPersonaRef) {
		return true
	}
	if hasNonEmptyRefList(bli, objects.FieldKeyPersonaRefs) {
		return true
	}
	if hasAnyStringRef(bli, objects.FieldKeyAssigneePersonaRef) {
		return true
	}
	if hasNonEmptyRefList(bli, objects.FieldKeyStakeholderRefs) {
		return true
	}
	if hasAnyStringRef(bli, objects.FieldKeyStakeholderType) {
		return true
	}
	if v := bli[objects.FieldKeyStakeholders]; v != nil {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t) != ""
		case []any:
			return len(t) > 0
		case []string:
			return len(t) > 0
		}
	}
	return false
}

func hasUnresolvedBlocker(bli map[string]any) bool {
	st, _ := bli[objects.FieldKeyStatus].(string)
	if strings.EqualFold(strings.TrimSpace(st), objects.ObjectStatusBlocked) {
		return true
	}
	return hasNonEmptyRefList(bli, objects.FieldKeyRiskBlockerRefs)
}

func hasAnyStringRef(obj map[string]any, key string) bool {
	s, _ := obj[key].(string)
	return strings.TrimSpace(s) != ""
}

func hasNonEmptyRefList(obj map[string]any, key string) bool {
	v := obj[key]
	if v == nil {
		return false
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) != ""
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, s := range t {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}
