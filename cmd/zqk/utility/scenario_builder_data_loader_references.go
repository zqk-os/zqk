package utility

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/validation"
)

// resolveReference resolves a single reference string using the ID stream
// Returns the resolved ID if found, otherwise returns the original reference
func resolveReference(refStr string, idStream map[string]string) string {
	if actualID, found := idStream[refStr]; found {
		return actualID
	}
	return refStr
}

// resolveReferenceList resolves a list of references using the ID stream
// Returns the resolved list and whether any references were resolved
func resolveReferenceList(refList []any, idStream map[string]string) ([]any, bool) {
	resolved := false
	newRefList := make([]any, 0, len(refList))
	for _, refItem := range refList {
		if refStr, ok := refItem.(string); ok && refStr != emptyValue {
			actualID := resolveReference(refStr, idStream)
			if actualID != refStr {
				resolved = true
			}
			newRefList = append(newRefList, actualID)
		} else {
			// Keep non-string items as-is
			newRefList = append(newRefList, refItem)
		}
	}
	return newRefList, resolved
}

// processReferenceField processes a single reference field (ending with "_ref")
// Returns true if the field was processed and updated
func processSingleReferenceField(obj map[string]any, fieldName string, fieldValue any, idStream map[string]string) bool {
	if !strings.HasSuffix(fieldName, "_ref") {
		return false
	}
	refStr, ok := fieldValue.(string)
	if !ok || refStr == emptyValue {
		return false
	}
	actualID := resolveReference(refStr, idStream)
	if actualID != refStr {
		obj[fieldName] = actualID
		return true
	}
	return false
}

// processReferenceListField processes a list reference field (ending with "_refs")
// Returns true if the field was processed and updated
func processReferenceListField(obj map[string]any, fieldName string, fieldValue any, idStream map[string]string) bool {
	if !strings.HasSuffix(fieldName, "_refs") {
		return false
	}
	refList, ok := fieldValue.([]any)
	if !ok {
		return false
	}
	newRefList, resolved := resolveReferenceList(refList, idStream)
	if resolved {
		obj[fieldName] = newRefList
		return true
	}
	return false
}

// resolveReferencesFromIDStream resolves placeholder IDs in reference fields using the ID stream
// This allows objects to reference other objects created in the same scenario build using placeholder IDs
// (e.g., "WS-001", "PRI-001") that are dynamically mapped to actual IDs as objects are created
func (sb *ScenarioBuilder) resolveReferencesFromIDStream(obj map[string]any, idStream map[string]string, idStreamMu *sync.Mutex) {
	idStreamMu.Lock()
	defer idStreamMu.Unlock()

	// Iterate through all fields in the object
	for fieldName, fieldValue := range obj {
		// Handle single reference fields (ending with "_ref")
		processSingleReferenceField(obj, fieldName, fieldValue, idStream)

		// Handle list reference fields (ending with "_refs")
		processReferenceListField(obj, fieldName, fieldValue, idStream)
	}
}

// collectReferenceStrings extracts all reference strings from a field value
// Handles both []any and []string types
func collectReferenceStrings(fieldValue any) []string {
	var refs []string
	if refList, ok := fieldValue.([]any); ok {
		for _, refItem := range refList {
			if refStr, ok := refItem.(string); ok && refStr != emptyValue {
				refs = append(refs, refStr)
			}
		}
	} else if refList, ok := fieldValue.([]string); ok {
		for _, refStr := range refList {
			if refStr != emptyValue {
				refs = append(refs, refStr)
			}
		}
	}
	return refs
}

// verifyReferencedObjectsExistMemoized verifies that all referenced objects exist using memoized cache
// This implements relaxed mode - allows forward references if objects are in the data file
// Note: idStreamMu is not locked here - referenceExistsMemoized handles its own locking
func (sb *ScenarioBuilder) verifyReferencedObjectsExistMemoized(ctx context.Context, obj map[string]any, kind string, idStream map[string]string, idStreamMu *sync.Mutex, referenceCache map[string]bool, referenceCacheMu *sync.Mutex) error {
	// Extract all reference fields from object
	var missingRefs []string

	// Iterate through all fields to find reference fields
	for fieldName, fieldValue := range obj {
		// Handle single reference fields (ending with "_ref")
		if strings.HasSuffix(fieldName, "_ref") {
			if refStr, ok := fieldValue.(string); ok && refStr != emptyValue {
				if !sb.referenceExistsMemoized(ctx, refStr, idStream, idStreamMu, referenceCache, referenceCacheMu) {
					missingRefs = append(missingRefs, fmt.Sprintf("%s:%s", fieldName, refStr))
				}
			}
		}

		// Handle list reference fields (ending with "_refs")
		if strings.HasSuffix(fieldName, "_refs") {
			refs := collectReferenceStrings(fieldValue)
			for _, refStr := range refs {
				if !sb.referenceExistsMemoized(ctx, refStr, idStream, idStreamMu, referenceCache, referenceCacheMu) {
					missingRefs = append(missingRefs, fmt.Sprintf("%s:%s", fieldName, refStr))
				}
			}
		}
	}

	if len(missingRefs) > 0 {
		return errfmt.Errorf("referenced objects do not exist: %v", missingRefs)
	}

	return nil
}

// referenceExistsMemoized checks if a referenced object exists using memoized cache
// Checks: 1) ID stream (already created), 2) Reference cache (memoized), 3) Storage (fallback), 4) Data file (forward references)
// This implements relaxed mode - allows forward references if objects are in the data file
func (sb *ScenarioBuilder) referenceExistsMemoized(ctx context.Context, refID string, idStream map[string]string, idStreamMu *sync.Mutex, referenceCache map[string]bool, referenceCacheMu *sync.Mutex) bool {
	// Check 1: ID stream (already created in this batch)
	idStreamMu.Lock()
	_, existsInStream := idStream[refID]
	idStreamMu.Unlock()
	if existsInStream {
		return true
	}

	// Check 2: Reference cache (memoized from data file)
	referenceCacheMu.Lock()
	existsInCache, cached := referenceCache[refID]
	referenceCacheMu.Unlock()
	if cached && existsInCache {
		return true
	}

	// Check 3: Storage (object already exists) - cache result
	if exists, err := sb.storage.Exists(ctx, sb.secCtx, refID); err == nil && exists {
		// Memoize the result
		referenceCacheMu.Lock()
		referenceCache[refID] = true
		referenceCacheMu.Unlock()
		return true
	}

	// Check 4: Data file (forward references) - relaxed mode allows forward references
	// Infer kind from referenced ID (e.g., PRI-001 -> priority_plan)
	// Then check if any object of that kind exists in the data file
	// This allows references to objects that will be created later in the batch
	idValidator := validation.GetIDValidator()
	if idValidator != nil {
		if err := idValidator.LoadPatterns(); err == nil {
			refKind := idValidator.InferKindFromID(refID)
			if refKind != emptyValue {
				// Check if any object of this kind exists in data file (forward reference)
				for _, obj := range sb.dataFileObjects {
					if objKind, ok := obj[objects.FieldKeyKind].(string); ok && objKind == refKind {
						// Object of referenced kind exists in data file - allow forward reference
						// Also check if it has the exact ID (in case it was set)
						if objID, ok := obj[objects.FieldKeyID].(string); ok && objID == refID {
							// Exact match - cache it
							referenceCacheMu.Lock()
							referenceCache[refID] = true
							referenceCacheMu.Unlock()
							return true
						}
						// Kind matches - allow forward reference (object will be created)
						// Cache the forward reference
						referenceCacheMu.Lock()
						referenceCache[refID] = true
						referenceCacheMu.Unlock()
						return true
					}
				}
			}
		}
	}

	// Not found - memoize negative result to avoid repeated checks
	referenceCacheMu.Lock()
	referenceCache[refID] = false
	referenceCacheMu.Unlock()
	return false
}
