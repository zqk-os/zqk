package bldr_instance_v1

import (
	"sort"

	"github.com/zqk-os/zqk/pkg/objects"
)

const specFileExtYAML = ".yaml"

// systemFields holds both the order and lookup map for system fields
type systemFields struct {
	order  []string
	lookup map[string]bool
}

// getSystemFields returns the system fields object (single source of truth)
var getSystemFields = func() *systemFields {
	order := []string{
		objects.FieldKeyID,
		objects.FieldKeyKind,
		objects.FieldKeySchemaVersion,
		objects.FieldKeyCreatedAt,
		objects.FieldKeyCreatedBy,
		objects.FieldKeyUpdatedAt,
		objects.FieldKeyUpdatedBy,
		objects.FieldKeyNamespaceID,
		objects.FieldKeyStatus,
	}
	lookup := make(map[string]bool, len(order))
	for _, field := range order {
		lookup[field] = true
	}
	return &systemFields{
		order:  order,
		lookup: lookup,
	}
}()

// systemFieldOrder returns the canonical order of system fields (always first)
func systemFieldOrder() []string {
	return getSystemFields.order
}

// isSystemField checks if a field is a system field
func isSystemField(fieldName string) bool {
	return getSystemFields.lookup[fieldName]
}

// buildFieldOrderFromSpec builds canonical field order from spec (not hardcoded)
// System fields first, then spec-defined fields in sorted order
// This avoids hardcoding field order and ensures it stays in sync with spec changes
func buildFieldOrderFromSpec(ontology string) []string {
	// Start with system fields (constant order)
	order := make([]string, 0, len(systemFieldOrder())+10)
	order = append(order, systemFieldOrder()...)

	// Load spec and add spec-defined fields in sorted order
	// Use ResolvedFields to include inherited fields (e.g., from base_object, auditable)
	specLoader := objects.GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance(ontology + specFileExtYAML)
	if err == nil && spec != nil && spec.ResolvedFields != nil {
		fieldNames := make([]string, 0, len(spec.ResolvedFields))
		for fieldName := range spec.ResolvedFields {
			// Skip system fields that are already in order
			if !isSystemField(fieldName) {
				fieldNames = append(fieldNames, fieldName)
			}
		}
		sort.Strings(fieldNames)
		order = append(order, fieldNames...)
	}

	return order
}
