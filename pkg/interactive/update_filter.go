package interactive

import "github.com/zqk-os/zqk/pkg/objects"

// UpdateFieldFilter filters fields for UPDATE operations
// Unlike CREATE, UPDATE allows modifying mutable fields but not immutable fields
type UpdateFieldFilter struct{}

// NewUpdateFieldFilter creates a new filter for update operations
func NewUpdateFieldFilter() *UpdateFieldFilter {
	return &UpdateFieldFilter{}
}

// IsFieldImmutable checks if a field is immutable (cannot be updated)
func (f *UpdateFieldFilter) IsFieldImmutable(fieldName string) bool {
	// Immutable fields that cannot be updated
	immutableFields := map[string]bool{
		objects.FieldKeyID:            true, // ID cannot change
		objects.FieldKeyKind:          true, // Kind cannot change
		objects.FieldKeyCreatedAt:     true, // Creation timestamp is immutable
		objects.FieldKeyCreatedBy:     true, // Creator is immutable
		objects.FieldKeySchemaVersion: true, // Schema version is system-managed
	}

	return immutableFields[fieldName]
}

// IsFieldAutoManaged checks if a field is auto-managed (updated automatically, not by user)
func (f *UpdateFieldFilter) IsFieldAutoManaged(fieldName string) bool {
	// Auto-managed fields (updated by system, not user)
	autoManagedFields := map[string]bool{
		objects.FieldKeyUpdatedAt: true, // Updated automatically on save
		objects.FieldKeyUpdatedBy: true, // Updated automatically from security context
	}

	return autoManagedFields[fieldName]
}

// IsFieldUpdatable checks if a field can be updated by the user
func (f *UpdateFieldFilter) IsFieldUpdatable(fieldName string) bool {
	// Not updatable if immutable
	if f.IsFieldImmutable(fieldName) {
		return false
	}

	// Auto-managed fields are technically updatable but will be overwritten
	// We allow them in the template but they'll be auto-set on save
	return true
}
