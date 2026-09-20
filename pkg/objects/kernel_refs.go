package objects

import "strings"

// kernelRefQueryTraitIndividuals are the query traits field_reference_group expands to.
// Specs must declare the group, not this list.
var kernelRefQueryTraitIndividuals = []string{
	"readable", "filterable", "groupable", "searchable", "sortable",
}

// FieldChecklistSecurityIsSensitive reports checklist.security labels that must
// not receive field_reference_group (PII / confidential pointer fields).
// "non-sensitive" is never sensitive, even though the substring "sensitive" appears.
func FieldChecklistSecurityIsSensitive(security string) bool {
	s := strings.ToLower(strings.TrimSpace(security))
	if s == "" || strings.Contains(s, "non-sensitive") {
		return false
	}
	for _, tok := range []string{"pii", "confidential", "sensitive", "secret"} {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}

// Non-kernel leftover names that historically used the _ref suffix.
// _ref / _refs is reserved for kernel object IDs. These must not appear in
// object_specs; they remain here so list/filter do not treat git/file/token
// leftovers as graph edges.
var nonKernelRefFieldNames = map[string]struct{}{
	"branch_ref":      {},
	"commit_ref":      {},
	"commit_refs":     {},
	"lifecycle_ref":   {},
	"lifecycle_refs":  {},
	"capability_refs": {},
	"document_refs":   {}, // leftover mixed path/ID list; kernel pointers are doc_entry_refs
}

// IsObjectRefFieldName reports whether name uses the kernel-object pointer suffix.
func IsObjectRefFieldName(name string) bool {
	return strings.HasSuffix(name, "_refs") || strings.HasSuffix(name, "_ref")
}

// IsKernelObjectRefField reports a live kernel-object pointer (singular or list).
func IsKernelObjectRefField(name string) bool {
	if !IsObjectRefFieldName(name) {
		return false
	}
	_, banned := nonKernelRefFieldNames[name]
	return !banned
}

// IsLeftoverNonKernelRefField reports a historical *_ref name that is not a kernel pointer.
func IsLeftoverNonKernelRefField(name string) bool {
	return IsObjectRefFieldName(name) && !IsKernelObjectRefField(name)
}

// KernelRefFieldPair returns the singular and plural names for a ref field.
func KernelRefFieldPair(name string) (singular, plural string, ok bool) {
	if !IsKernelObjectRefField(name) {
		return "", "", false
	}
	if strings.HasSuffix(name, "_refs") {
		return strings.TrimSuffix(name, "s"), name, true
	}
	return name, name + "s", true
}

// KernelObjectRefIDs collects non-empty IDs from the singular and/or plural
// forms of a kernel ref field (deduped, first-seen order).
func KernelObjectRefIDs(obj map[string]any, field string) []string {
	if obj == nil {
		return nil
	}
	singular, plural, ok := KernelRefFieldPair(field)
	if !ok {
		return nil
	}
	return collectRefIDs(obj[singular], obj[plural], leftoverKernelRefPlural(obj, plural))
}

func leftoverKernelRefPlural(obj map[string]any, plural string) any {
	if obj == nil || plural != FieldKeyDocEntryRefs {
		return nil
	}
	return obj[FieldKeyDocumentRefs]
}

// KernelObjectRefContains reports whether want is among the bound IDs for field.
func KernelObjectRefContains(obj map[string]any, field, want string) bool {
	return refIDsContain(KernelObjectRefIDs(obj, field), want)
}

// KernelObjectRefGroupKey is the --group-by key for a kernel ref field:
// one id, or comma-joined ids when the object sits in multiple targets.
func KernelObjectRefGroupKey(obj map[string]any, field string) string {
	return strings.Join(KernelObjectRefIDs(obj, field), ",")
}

func collectRefIDs(values ...any) []string {
	var ids []string
	seen := make(map[string]struct{})
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || strings.Contains(s, "/") {
			return
		}
		if _, dup := seen[s]; dup {
			return
		}
		seen[s] = struct{}{}
		ids = append(ids, s)
	}
	for _, value := range values {
		switch v := value.(type) {
		case string:
			add(v)
		case []string:
			for _, id := range v {
				add(id)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		}
	}
	return ids
}

func (p *ParsedObject) typedRefList(plural string) []string {
	if p == nil {
		return nil
	}
	switch plural {
	case FieldKeyWorkstreamRefs:
		return p.WorkstreamRefs
	case FieldKeyGoalRefs:
		return p.GoalRefs
	case FieldKeyMilestoneRefs:
		return p.MilestoneRefs
	case FieldKeyRequirementRefs:
		return p.RequirementRefs
	case FieldKeyBacklogItemRefs:
		return p.BacklogItemRefs
	case FieldKeyCriteriaRefs:
		return p.CriteriaRefs
	case FieldKeyTestCaseRefs:
		return p.TestCaseRefs
	case FieldKeyDocEntryRefs:
		return p.DocEntryRefs
	default:
		return nil
	}
}

// KernelObjectRefIDs returns bound kernel object IDs for field from typed slices and raw aliases.
func (p *ParsedObject) KernelObjectRefIDs(field string) []string {
	if p == nil {
		return nil
	}
	singular, plural, ok := KernelRefFieldPair(field)
	if !ok {
		return nil
	}
	var rawSingular, rawPlural any
	if p.Raw != nil {
		rawSingular = p.Raw[singular]
		rawPlural = p.Raw[plural]
	}
	if typed := p.typedRefList(plural); len(typed) > 0 {
		rawPlural = typed
	}
	var leftover any
	if p.Raw != nil {
		leftover = leftoverKernelRefPlural(p.Raw, plural)
	}
	return collectRefIDs(rawSingular, rawPlural, leftover)
}

func kernelRefFieldValue(ids []string, name string) (any, bool) {
	if len(ids) == 0 {
		return nil, false
	}
	if strings.HasSuffix(name, "_refs") || len(ids) > 1 {
		return ids, true
	}
	return ids[0], true
}

func refIDsContain(ids []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
