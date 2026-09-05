package storage

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/migration/parser"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// ShouldCascadeDeleteDependent determines if a dependent object should be cascade-deleted.
// If the dependent references the deleted object via a field with 'composition' criticality,
// it is considered a parent object and should NOT be cascade-deleted (returns false).
// Otherwise, it returns true.
func ShouldCascadeDeleteDependent(dependent map[string]any, deletedID string) bool {
	if dependent == nil || deletedID == emptyValue {
		return true
	}

	kind, _ := dependent[objects.FieldKeyKind].(string)
	if kind == emptyValue {
		return true
	}

	fr := objects.GetGlobalFieldRegistry()
	fieldDefs, loaded := fr.GetFieldsForKindIfLoaded(kind)
	if !loaded || fieldDefs == nil {
		return true // Default to cascade
	}

	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(dependent)

	for fieldName, refValue := range refFields {
		if fieldName == "commit_refs" {
			continue
		}

		isMatch := false
		switch v := refValue.(type) {
		case string:
			isMatch = ReferenceStringMatchesObjectID(v, deletedID)
		case []any:
			for _, item := range v {
				if str, ok := item.(string); ok && ReferenceStringMatchesObjectID(str, deletedID) {
					isMatch = true
					break
				}
			}
		case []string:
			for _, str := range v {
				if ReferenceStringMatchesObjectID(str, deletedID) {
					isMatch = true
					break
				}
			}
		}

		if isMatch {
			baseFieldName := strings.Split(fieldName, "[")[0]
			baseFieldName = strings.Split(baseFieldName, ".")[0]

			for _, fieldInfo := range fieldDefs.AllFields {
				if fieldInfo.Name == baseFieldName {
					criticality := strings.ToLower(fieldInfo.Criticality)
					if criticality == "composition" {
						return false // It's a parent! Do not cascade delete.
					}
					if criticality == "association" {
						return false // Loosely coupled! Do not cascade delete, just unlink.
					}
					break
				}
			}
		}
	}

	return true
}

// ReferenceStringMatchesObjectID reports whether refStr points at targetID (namespace, kind:id, or bare ID).
func ReferenceStringMatchesObjectID(refStr, targetID string) bool {
	if refStr == emptyValue || targetID == emptyValue {
		return false
	}
	parsed := validation.ParseNamespace(refStr)
	if parsed != nil && parsed.ObjectID == targetID {
		return true
	}
	if strings.Contains(refStr, ":") {
		parts := strings.SplitN(refStr, ":", 2)
		if len(parts) == 2 && parts[1] == targetID {
			return true
		}
		parts = strings.Split(refStr, ":")
		if len(parts) > 0 && parts[len(parts)-1] == targetID {
			return true
		}
	}
	return refStr == targetID
}

// StripReferenceFieldsRemovingID removes targetID from reference-valued fields on obj (in-memory).
// Skips commit_refs (Git hashes). Returns field updates suitable for ObjectStorageProvider.Update.
func StripReferenceFieldsRemovingID(obj map[string]any, targetID string) map[string]any {
	if obj == nil || targetID == emptyValue {
		return nil
	}
	yamlParser := parser.NewYAMLParser()
	refFields := yamlParser.ExtractReferenceFields(obj)
	updates := make(map[string]any)
	for fieldName, refValue := range refFields {
		if fieldName == "commit_refs" {
			continue
		}
		switch v := refValue.(type) {
		case string:
			if ReferenceStringMatchesObjectID(v, targetID) {
				updates[fieldName] = ""
			}
		case []any:
			out := stripAnySliceRemovingID(v, targetID)
			if len(out) != len(v) {
				updates[fieldName] = out
			}
		case []string:
			out := stripStringSliceRemovingID(v, targetID)
			if len(out) != len(v) {
				updates[fieldName] = out
			}
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return updates
}

func stripAnySliceRemovingID(in []any, targetID string) []any {
	out := make([]any, 0, len(in))
	for _, item := range in {
		str, ok := item.(string)
		if !ok || !ReferenceStringMatchesObjectID(str, targetID) {
			out = append(out, item)
		}
	}
	return out
}

func stripStringSliceRemovingID(in []string, targetID string) []string {
	out := make([]string, 0, len(in))
	for _, str := range in {
		if !ReferenceStringMatchesObjectID(str, targetID) {
			out = append(out, str)
		}
	}
	return out
}

// UnlinkWouldStripArchivedCriteriaLineage reports whether removing deletedID
// from dep would erase criteria history that complete/archived BLIs and
// milestones must keep (archived-only lineage satisfies the complete gate;
// never delete those CRITs to “heal” it).
func UnlinkWouldStripArchivedCriteriaLineage(dep map[string]any, deletedID string) error {
	if dep == nil || deletedID == emptyValue {
		return nil
	}
	if !strings.HasPrefix(deletedID, "CRIT-") {
		return nil
	}
	depKind := objects.GetString(dep, objects.FieldKeyKind)
	depStatus := objects.GetString(dep, objects.FieldKeyStatus)
	if (depKind != objects.KindBacklogItem && depKind != objects.KindMilestone) ||
		(depStatus != objects.ObjectStatusComplete && depStatus != objects.ObjectStatusArchived) {
		return nil
	}
	depID := objects.GetString(dep, objects.FieldKeyID)
	return errfmt.Errorf("cannot unlink archived criteria lineage %s from %s %s with status %s", deletedID, depKind, depID, depStatus)
}

// UnlinkReferencesFromDependents strips deletedID from each dependent object’s reference fields.
func UnlinkReferencesFromDependents(ctx context.Context, secCtx *pkgctx.SecurityContext, store ObjectStorageProvider, deletedID string, dependentIDs []string) error {
	updateCtx := pkgctx.WithLifecycleBreakGlassReason(ctx, "system: unlink references before delete")
	for _, depID := range dependentIDs {
		obj, err := store.Read(ctx, secCtx, depID)
		if err != nil || obj == nil {
			continue
		}
		if err := UnlinkWouldStripArchivedCriteriaLineage(obj, deletedID); err != nil {
			return err
		}
		updates := StripReferenceFieldsRemovingID(obj, deletedID)
		if len(updates) == 0 {
			continue
		}
		if err := store.Update(updateCtx, secCtx, depID, updates); err != nil {
			return errfmt.Errorf(ConstMiscUnlinkReferencesFromDependentSW, depID, err)
		}
	}
	return nil
}
