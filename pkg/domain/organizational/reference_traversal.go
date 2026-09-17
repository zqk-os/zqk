package organizational

import (
	"context"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

const emptyRefValue = ""

// findAffectedZqkObjects finds ZQK objects that reference the affected organizational objects
//
//nolint:unparam // Keep error return for future/consistency; currently best-effort and returns nil.
func findAffectedZqkObjects(ctx context.Context, storageProvider storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, affectedObjects map[string]any) (map[string]any, error) {
	result := make(map[string]any)
	storageCtx := pkgctx.NewStorageContext()

	// Extract organizational object IDs from affected_objects map
	orgObjectIDs := extractOrgObjectIDs(affectedObjects)

	// Find workstreams that reference these organizational objects
	if workstreams, err := findObjectsReferencingOrgObjects(ctx, storageProvider, objects.KindWorkstream, orgObjectIDs, secCtx, storageCtx); err == nil {
		if len(workstreams) > 0 {
			result["workstreams"] = workstreams
		}
	}

	// Find goals that reference these organizational objects
	if goals, err := findObjectsReferencingOrgObjects(ctx, storageProvider, objects.KindGoal, orgObjectIDs, secCtx, storageCtx); err == nil {
		if len(goals) > 0 {
			result["goals"] = goals
		}
	}

	// Find backlog_items that reference these organizational objects
	if backlogItems, err := findObjectsReferencingOrgObjects(ctx, storageProvider, objects.KindBacklogItem, orgObjectIDs, secCtx, storageCtx); err == nil {
		if len(backlogItems) > 0 {
			result["backlog_items"] = backlogItems
		}
	}

	// Find milestones that reference these organizational objects
	if milestones, err := findObjectsReferencingOrgObjects(ctx, storageProvider, objects.KindMilestone, orgObjectIDs, secCtx, storageCtx); err == nil {
		if len(milestones) > 0 {
			result["milestones"] = milestones
		}
	}

	return result, nil
}

// extractOrgObjectIDs extracts all organizational object IDs from the affected_objects map
func extractOrgObjectIDs(affectedObjects map[string]any) []string {
	var ids []string

	for _, objList := range affectedObjects {
		if list, ok := objList.([]any); ok {
			for _, item := range list {
				if id, ok := item.(string); ok && id != emptyRefValue {
					ids = append(ids, id)
				}
			}
		}
	}

	return ids
}

// findObjectsReferencingOrgObjects finds objects of a specific kind that reference any of the given organizational object IDs
func findObjectsReferencingOrgObjects(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	kind string,
	orgObjectIDs []string,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
) ([]any, error) {
	if len(orgObjectIDs) == 0 {
		return []any{}, nil
	}

	// List all objects of this kind
	filter := storage.ListFilter{
		Kind: kind,
	}
	result, err := storageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Errorf("failed to list %s objects: %w", kind, err)
	}

	// Filter objects that reference any of the organizational object IDs
	var matching []any
	orgIDSet := make(map[string]bool)
	for _, id := range orgObjectIDs {
		orgIDSet[id] = true
		// Also match namespace IDs (e.g., "domain:organizational:division:DIV-001")
		if nsID := extractNamespaceID(id); nsID != emptyRefValue && nsID != id {
			orgIDSet[nsID] = true
		}
	}

	// Determine which field to check based on object kind
	referenceFields := getOrgReferenceFields(kind)

	for _, obj := range result.Objects {
		if referencesAnyOrgObject(obj, orgIDSet, referenceFields) {
			objID, _ := obj[objects.FieldKeyID].(string)
			if objID != emptyRefValue {
				matching = append(matching, objID)
			}
		}
	}

	return matching, nil
}

// extractNamespaceID extracts namespace ID from a reference (handles both ID and namespace formats)
func extractNamespaceID(ref string) string {
	// If already in namespace format, return as-is
	if strings.Contains(ref, ":") {
		return ref
	}
	// Otherwise, try to infer namespace (simplified - could be enhanced)
	return ref
}

// getOrgReferenceFields returns the field names that might contain organizational references for a given kind
func getOrgReferenceFields(kind string) []string {
	refs := []string{objects.FieldKeyDivisionRef, "team_ref"}
	switch kind {
	case objects.KindWorkstream:
		return refs
	case objects.KindGoal:
		return refs
	case objects.KindBacklogItem:
		return refs
	case objects.KindMilestone:
		return refs
	default:
		return []string{}
	}
}

// referencesAnyOrgObject checks if an object references any of the given organizational object IDs
func referencesAnyOrgObject(obj map[string]any, orgIDSet map[string]bool, referenceFields []string) bool {
	for _, field := range referenceFields {
		if refValue, ok := obj[field]; ok && refValue != nil {
			switch
			// Handle single reference (string)
			v := refValue.(type) {
			case string:
				if orgIDSet[v] || orgIDSet[extractIDFromNamespace(v)] {
					return true
				}
			case

				// Handle list of references
				[]any:
				for _, item := range v {
					if refStr, ok := item.(string); ok {
						if orgIDSet[refStr] || orgIDSet[extractIDFromNamespace(refStr)] {
							return true
						}
					}
				}
			}
		}

	}
	return false
}

// extractIDFromNamespace extracts the ID portion from a namespace-formatted reference
func extractIDFromNamespace(namespace string) string {
	// Handle format like "domain:organizational:division:DIV-001" -> "DIV-001"
	parts := strings.Split(namespace, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return namespace
}
