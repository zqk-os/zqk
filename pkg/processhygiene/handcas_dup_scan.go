package processhygiene

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// Rule IDs emitted by the hand-CAS / duplicate-ID scans.
const (
	RuleIDDuplicateIDOrphan  = "duplicate_id_orphan"
	RuleIDHandCASMaterialize = "hand_cas_materialize"
)

// Finding descriptions state the hygiene rule in plain language. They deliberately do not cite
// kernel object IDs (decisions, backlog items): those objects can be archived, renamed, or
// deleted, which would leave a scan report pointing at a reference nobody can resolve.
const (
	descDuplicateIDOrphan  = "duplicate object ID %q with count=%d; the previous CAS entry is orphaned — every mutation must use a unique object ID"
	descHandCASOwnerPrefix = "detected hand-CAS materialization pattern (owner_ref has placeholder prefix %q; draft-to-CAS promotion must go through the object API)"
	descHandCASFakeAccount = "detected placeholder account ID %q used as an object ID (no real agent suffix; draft-to-CAS promotion must go through the object API)"
)

// placeholderOwnerRefPrefixes and placeholderAccountIDs are the known placeholder-account shapes
// that hand-CAS materialization leaves behind. They are data, not inline conditionals, so a new
// shape is a one-line change; project-specific additions belong in the declarative rules file
// (.zqk/specs/process_hygiene_rules.yaml — see default_rules.yaml) rather than here.
var (
	placeholderOwnerRefPrefixes = []string{
		"ACC-901-placeholder", // canonical placeholder owner_ref from hand-CAS incidents
		"FAKE-OWNER-",         // generic fixture pattern for unauthorized ownership
	}
	placeholderAccountIDs = []string{
		"ACC-901", // numeric placeholder with no real agent suffix
	}
)

// ScanObjectsForDupIDs scans the given object list for duplicate IDs.
// Returns findings with RuleID [RuleIDDuplicateIDOrphan] for each duplicated ID.
func ScanObjectsForDupIDs(objectList []map[string]any) []Finding {
	idCounts := make(map[string]int)
	idKinds := make(map[string]string) // track first kind seen per ID

	for _, obj := range objectList {
		id, _ := obj[objects.FieldKeyID].(string)
		if strings.TrimSpace(id) == emptyValue {
			continue
		}
		idCounts[id]++
		if idKinds[id] == emptyValue {
			if k, ok := obj[objects.FieldKeyKind].(string); ok && strings.TrimSpace(k) != emptyValue {
				idKinds[id] = k
			}
		}
	}

	var findings []Finding
	for id, count := range idCounts {
		if count > 1 {
			findings = append(findings, Finding{
				RuleID:          RuleIDDuplicateIDOrphan,
				RuleDescription: fmt.Sprintf(descDuplicateIDOrphan, id, count),
				ID:              id,
				Kind:            idKinds[id],
				Detail:          fmt.Sprintf("Found %d occurrences of duplicate ID", count),
			})
		}
	}
	return findings
}

// ScanObjectsForHandCASMate scans the given object list for hand-CAS materialization markers.
// It checks owner_ref and id for placeholder or fake account references that indicate
// unauthorized manual draft-to-CAS promotion.
func ScanObjectsForHandCASMate(objectList []map[string]any) []Finding {
	var findings []Finding
	reported := make(map[string]bool) // one finding per (object, matched marker)

	for _, obj := range objectList {
		id, _ := obj[objects.FieldKeyID].(string)
		kind, _ := obj[objects.FieldKeyKind].(string)
		if id == emptyValue || kind == emptyValue {
			continue
		}
		if f, ok := placeholderOwnerRefFinding(obj, id, kind, reported); ok {
			findings = append(findings, f)
		}
		if f, ok := placeholderObjectIDFinding(id, kind, reported); ok {
			findings = append(findings, f)
		}
	}

	return findings
}

// placeholderOwnerRefFinding reports the first placeholder prefix carried by the object's owner_ref.
func placeholderOwnerRefFinding(obj map[string]any, id, kind string, reported map[string]bool) (Finding, bool) {
	ownerRef := getAnyStringField(obj, objects.FieldKeyOwnerRef)
	for _, prefix := range placeholderOwnerRefPrefixes {
		if !strings.HasPrefix(ownerRef, prefix) {
			continue
		}
		key := id + "-" + objects.FieldKeyOwnerRef + "-" + prefix
		if reported[key] {
			return Finding{}, false
		}
		reported[key] = true
		return Finding{
			RuleID:          RuleIDHandCASMaterialize,
			RuleDescription: fmt.Sprintf(descHandCASOwnerPrefix, prefix),
			ID:              id,
			Kind:            kind,
			Field:           objects.FieldKeyOwnerRef,
			Value:           ownerRef,
		}, true
	}
	return Finding{}, false
}

// placeholderObjectIDFinding reports an object whose own ID is a known placeholder account.
func placeholderObjectIDFinding(id, kind string, reported map[string]bool) (Finding, bool) {
	for _, placeholder := range placeholderAccountIDs {
		if id != placeholder && !strings.HasPrefix(id, placeholder+"/") {
			continue
		}
		key := objects.FieldKeyID + ":" + placeholder
		if reported[key] {
			return Finding{}, false
		}
		reported[key] = true
		return Finding{
			RuleID:          RuleIDHandCASMaterialize,
			RuleDescription: fmt.Sprintf(descHandCASFakeAccount, placeholder),
			ID:              id,
			Kind:            kind,
			Field:           objects.FieldKeyID,
		}, true
	}
	return Finding{}, false
}

// ScanAllObjects combines all hygiene checks in one pass.
// This is the main entry point used at runtime before object persistence.
func ScanAllObjects(objectList []map[string]any) []Finding {
	findings := ScanObjectsForDupIDs(objectList)
	findings = append(findings, ScanObjectsForHandCASMate(objectList)...)
	sortFindings(findings)
	return findings
}

func getAnyStringField(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok {
		return emptyValue
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// sortFindings sorts findings in-place by rule ID then object ID for deterministic output.
func sortFindings(findings []Finding) {
	slices.SortFunc(findings, func(a, b Finding) int {
		if a.RuleID != b.RuleID {
			return strings.Compare(a.RuleID, b.RuleID)
		}
		return strings.Compare(a.ID, b.ID)
	})
}
