package mcp

import (
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

// applyObjectAccessControl applies object-level access control to command results
// Filters objects based on spec-based access control
// Uses permission cache if available for optimized access control
func applyObjectAccessControl(result map[string]any, secCtx *pkgctx.SecurityContext, permissionCache any) map[string]any {
	if secCtx == nil {
		return result
	}

	// Get permission cache if available
	pc := getPermissionCache(permissionCache)
	if pc == nil {
		return result // Skip access control if no cache (backward compatibility)
	}

	// Fast path: Check if user is active
	if !pc.IsUserActive(secCtx.AccountID) {
		return map[string]any{
			"error":      "user access deactivated",
			"error_code": AccountInactive,
			"error_type": "account_inactive",
		}
	}

	// Get spec loader and create access control
	oac := createObjectAccessControl(pc)
	if oac == nil {
		return result
	}

	// Apply access control based on result format
	result = applyAccessControlToObjectsList(result, secCtx, pc, oac)
	result = applyAccessControlToSingleObject(result, secCtx, pc, oac)
	result = applyAccessControlToDirectObject(result, secCtx, pc, oac)

	return result
}

// getPermissionCache extracts permission cache from interface
func getPermissionCache(permissionCache any) *PermissionCache {
	if permissionCache == nil {
		return nil
	}
	if pc, ok := permissionCache.(*PermissionCache); ok {
		return pc
	}
	return nil
}

// createObjectAccessControl creates object access control from permission cache
func createObjectAccessControl(pc *PermissionCache) *ObjectAccessControl {
	specLoader := pc.GetSpecLoaderTyped()
	if specLoader == nil {
		return nil
	}
	eventEmitter := pc.GetEventEmitter()
	return NewObjectAccessControl(specLoader, eventEmitter)
}

// applyAccessControlToObjectsList applies access control to result format: {"objects": [...]}
func applyAccessControlToObjectsList(result map[string]any, secCtx *pkgctx.SecurityContext, pc *PermissionCache, oac *ObjectAccessControl) map[string]any {
	objects, ok := result["objects"].([]any)
	if !ok {
		return result
	}

	filtered := make([]any, 0, len(objects))
	for _, obj := range objects {
		objMap, ok := obj.(map[string]any)
		if !ok {
			filtered = append(filtered, obj)
			continue
		}

		// Check access and filter fields
		if filteredObj := filterObjectWithAccessControl(objMap, secCtx, pc, oac); filteredObj != nil {
			filtered = append(filtered, filteredObj)
		}
	}

	result["objects"] = filtered
	updateObjectCount(result, len(filtered))
	return result
}

// applyAccessControlToSingleObject applies access control to result format: {"object": {...}}
func applyAccessControlToSingleObject(result map[string]any, secCtx *pkgctx.SecurityContext, pc *PermissionCache, oac *ObjectAccessControl) map[string]any {
	obj, ok := result["object"].(map[string]any)
	if !ok {
		return result
	}

	// Check access
	hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(obj, secCtx)
	if !hasAccess {
		delete(result, "object")
		result["error"] = "access denied"
		result["error_code"] = AccessDenied
		result["error_type"] = "access_denied"
		return result
	}

	// Check spec-based access if needed
	if shouldCheckSpec && !oac.HasAccess(obj, secCtx) {
		delete(result, "object")
		result["error"] = "access denied"
		result["error_code"] = AccessDenied
		result["error_type"] = "access_denied"
		return result
	}

	// Apply field-level filtering
	result["object"] = oac.specAccessControl.FilterObjectFields(obj, secCtx, "read")
	return result
}

// applyAccessControlToDirectObject applies access control to result format: direct object (has "id" field)
func applyAccessControlToDirectObject(result map[string]any, secCtx *pkgctx.SecurityContext, pc *PermissionCache, oac *ObjectAccessControl) map[string]any {
	id, hasID := result[objects.FieldKeyID].(string)
	if !hasID {
		return result
	}

	// Check access
	hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(result, secCtx)
	if !hasAccess {
		return map[string]any{
			"error":            "access denied",
			"error_code":       AccessDenied,
			"error_type":       "access_denied",
			objects.FieldKeyID: id,
		}
	}

	// Check spec-based access if needed
	if shouldCheckSpec && !oac.HasAccess(result, secCtx) {
		return map[string]any{
			"error":            "access denied",
			"error_code":       AccessDenied,
			"error_type":       "access_denied",
			objects.FieldKeyID: id,
		}
	}

	// Apply field-level filtering
	return oac.specAccessControl.FilterObjectFields(result, secCtx, "read")
}

// filterObjectWithAccessControl checks access and filters object fields
// Returns nil if access is denied, otherwise returns filtered object
func filterObjectWithAccessControl(objMap map[string]any, secCtx *pkgctx.SecurityContext, pc *PermissionCache, oac *ObjectAccessControl) map[string]any {
	// Fast path: Check privilege tags first
	hasAccess, shouldCheckSpec := pc.HasObjectAccessFast(objMap, secCtx)
	if !hasAccess {
		return nil // Denied by privilege tags
	}

	// Check spec-based access if needed
	if shouldCheckSpec && !oac.HasAccess(objMap, secCtx) {
		return nil // Denied by spec
	}

	// Apply field-level filtering
	return oac.specAccessControl.FilterObjectFields(objMap, secCtx, "read")
}

// updateObjectCount updates the count field in result to match filtered objects
func updateObjectCount(result map[string]any, count int) {
	switch result["count"].(type) {
	case float64:
		result["count"] = float64(count)
	case int:
		result["count"] = count
	}
}
