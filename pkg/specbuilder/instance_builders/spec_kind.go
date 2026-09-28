package instance_builders

import (
	"sort"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

const specKindFileExtYAML = ".yaml"

// specKindSystemFields is the canonical prefix of every instance, ahead of spec fields.
var specKindSystemFields = func() struct {
	order  []string
	lookup map[string]bool
} {
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
	return struct {
		order  []string
		lookup map[string]bool
	}{order: order, lookup: lookup}
}()

// FieldOrderFromSpec returns system fields, then the spec's resolved fields in sorted order.
// A missing spec still returns the system prefix so callers can build an instance map.
func FieldOrderFromSpec(ontology string) []string {
	order := make([]string, 0, len(specKindSystemFields.order)+10)
	order = append(order, specKindSystemFields.order...)

	spec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance(ontology + specKindFileExtYAML)
	if err != nil || spec == nil || spec.ResolvedFields == nil {
		return order
	}
	fieldNames := make([]string, 0, len(spec.ResolvedFields))
	for fieldName := range spec.ResolvedFields {
		if !specKindSystemFields.lookup[fieldName] {
			fieldNames = append(fieldNames, fieldName)
		}
	}
	sort.Strings(fieldNames)
	return append(order, fieldNames...)
}

// SchemaVersionForKind returns the schema_version recorded on the kind's spec.
func SchemaVersionForKind(kind string) (string, error) {
	spec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance(kind + specKindFileExtYAML)
	if err != nil || spec == nil || spec.SchemaVersion == "" {
		return "", errfmt.Errorf("no spec found for kind: %s", kind)
	}
	return spec.SchemaVersion, nil
}

// NewForKind builds an instance from the live spec for kind.
// Callers use SetID, SetStatus, and SetField. Generated per-kind builders are not required.
func NewForKind(kind, schemaVersion string) InstanceBuilder {
	return NewBaseInstanceBuilder(kind, schemaVersion, FieldOrderFromSpec(kind), nil)
}
