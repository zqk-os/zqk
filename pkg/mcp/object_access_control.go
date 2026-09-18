package mcp

import (
	"slices"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// ObjectAccessControl handles object-level access restrictions
// Uses spec-based access control with field-level restrictions and cascading access
type ObjectAccessControl struct {
	specAccessControl *SpecAccessControl
	eventEmitter      *EventEmitter // Optional event emitter for permission events
}

// NewObjectAccessControl creates a new ObjectAccessControl instance
// Requires a spec loader for spec-based access control
// eventEmitter is optional for permission event emission
func NewObjectAccessControl(specLoader SpecLoader, eventEmitter ...*EventEmitter) *ObjectAccessControl {
	sac := NewSpecAccessControl(specLoader)
	oac := &ObjectAccessControl{
		specAccessControl: sac,
	}
	if len(eventEmitter) > 0 && eventEmitter[0] != nil {
		oac.eventEmitter = eventEmitter[0]
		sac.SetEventEmitter(eventEmitter[0])
	}
	return oac
}

// HasAccess checks if the security context has access to an object
// Uses spec-based access control with field-level restrictions
func (oac *ObjectAccessControl) HasAccess(obj map[string]any, secCtx *pkgctx.SecurityContext) bool {
	if secCtx == nil {
		return false
	}

	return oac.specAccessControl.HasObjectAccess(obj, secCtx)
}

// FilterObjectsByAccess filters a list of objects based on access restrictions
// Applies both object-level and field-level access control
func (oac *ObjectAccessControl) FilterObjectsByAccess(objects []map[string]any, secCtx *pkgctx.SecurityContext) []map[string]any {
	if secCtx == nil {
		// No security context = no access
		return []map[string]any{}
	}

	var filtered []map[string]any
	for _, obj := range objects {
		if oac.HasAccess(obj, secCtx) {
			// Apply field-level filtering
			filteredObj := oac.specAccessControl.FilterObjectFields(obj, secCtx, "read")
			filtered = append(filtered, filteredObj)
		}
	}
	return filtered
}

// AddAccessFilterToQuery adds access restriction filters to a query filter
// This can be used to filter at the database level for efficiency
// Currently returns existing filters - query-level filtering to be implemented
func (oac *ObjectAccessControl) AddAccessFilterToQuery(secCtx *pkgctx.SecurityContext, existingFilters map[string]any) map[string]any {
	if secCtx == nil {
		// No security context = no access
		// Return a filter that matches nothing
		return map[string]any{
			objects.FieldKeyID: map[string]any{
				"$eq": "__no_access__", // This will never match
			},
		}
	}

	// Check if admin (bypasses restrictions)
	if slices.Contains(secCtx.Roles, "admin") {
		// Admin has access to all - no filter needed
		return existingFilters
	}

	// Query-level filtering based on spec access rules to be implemented
	// For now, return existing filters and let FilterObjectsByAccess handle it in memory
	return existingFilters
}
