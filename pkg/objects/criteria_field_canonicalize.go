package objects

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// CanonicalizeCriteriaFieldKeys remaps kind-scoped synonyms onto category and
// drops the alias keys. Call only for kind criteria — FieldKeyType is a real
// field on other kinds. Invoked from CoerceMutationFields (every write path),
// not from generic object create/update CLI.
func CanonicalizeCriteriaFieldKeys(obj map[string]any) {
	if obj == nil {
		return
	}
	if !hasNonEmptyField(obj, FieldKeyCategory) {
		for _, alias := range []string{FieldKeyType, FieldKeyCriteriaType} {
			if v, ok := obj[alias]; ok && isNonEmptyAny(v) {
				obj[FieldKeyCategory] = v
				break
			}
		}
	}
	delete(obj, FieldKeyType)
	delete(obj, FieldKeyCriteriaType)
}

// RejectCriteriaRequirementRelatedRefs fails if related_object_refs contains a
// requirement id. Composition is parent-owned via requirement.criteria_refs.
// Production enforcement is overlay crit_no_req_related_refs (kernelcas compose).
// TRACK: BLI-KERNEL-REF-GRAPH-ACYCLIC-001.
func RejectCriteriaRequirementRelatedRefs(obj map[string]any) error {
	if obj == nil {
		return nil
	}
	v, ok := obj[FieldKeyRelatedObjectRefs]
	if !ok {
		return nil
	}
	for _, id := range criteriaRelatedRefIDs(v) {
		if strings.HasPrefix(id, "REQ-") {
			return errfmt.Errorf("criteria.related_object_refs must not contain requirements; composition is parent-owned via requirement.criteria_refs")
		}
	}
	return nil
}

func hasNonEmptyField(obj map[string]any, key string) bool {
	v, ok := obj[key]
	return ok && isNonEmptyAny(v)
}

func isNonEmptyAny(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) != emptyValue
	}
	return true
}

func criteriaRelatedRefIDs(v any) []string {
	switch t := v.(type) {
	case string:
		if t == emptyValue {
			return nil
		}
		return []string{t}
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != emptyValue {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
