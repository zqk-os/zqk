package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// FieldRegistryBuilder builds the field_registry spec at version v2_0_0
// File: bldr_v2/field_registry_builder.go - version is encoded in package/directory name
type FieldRegistryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewFieldRegistryBuilder creates a new builder for field_registry spec version v2_0_0
func NewFieldRegistryBuilder() *FieldRegistryBuilder {
	builder := &FieldRegistryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("field_registry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines the global mapping of string field keys to unique binary identifiers for physical compression.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addFieldRegistryFields()

	return builder
}

// addFieldRegistryFields adds the field_registry fields
func (b *FieldRegistryBuilder) addFieldRegistryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("active_fields", "list"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *FieldRegistryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *FieldRegistryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *FieldRegistryBuilder) GetOntology() string {
	return "field_registry"
}

func init() {
	builders.RegisterBuilder(NewFieldRegistryBuilder())
}
