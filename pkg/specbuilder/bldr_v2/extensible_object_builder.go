package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ExtensibleObjectBuilder builds the extensible_object spec at version v2_0_0
// File: bldr_v2/extensible_object_builder.go - version is encoded in package/directory name
type ExtensibleObjectBuilder struct {
	*builders.BaseSpecBuilder
}

// NewExtensibleObjectBuilder creates a new builder for extensible_object spec version v2_0_0
func NewExtensibleObjectBuilder() *ExtensibleObjectBuilder {
	builder := &ExtensibleObjectBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("extensible_object", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Base spec for external domain objects that integrate with Workstream OS. External domains must define their own specs that extend this base. By extending base_object, external domain objects automatically inherit: - All base object traits (listable, readable, searchable, filterable, etc.) - Base object fields (id, title, kind, status, schema_version, etc.) - Base object validation rules Domain-specific traits (like constrainable for components) are declared in the domain's own spec. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addExtensibleObjectFields()

	return builder
}

// addExtensibleObjectFields adds the extensible_object fields
func (b *ExtensibleObjectBuilder) addExtensibleObjectFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("domain", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for domain-specific routing").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("domain registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identifies the external domain this object belongs to").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"validation",
				"querying",
			}).
			Validation("Must be a registered domain").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"visualization",
				"ui",
				"api",
				"integration",
				"custom",
			}).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("EXT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("lifecycle_file", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for lifecycle loading").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("lifecycle system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Explicit reference to lifecycle definition file (optional - defaults to naming convention)").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle loading",
				"validation",
			}).
			Validation("Must reference valid lifecycle file if provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\.zqk/specs/lifecycles/.*\.yaml$`).
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("EXT-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("spec_context_broker", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("routes to correct broker").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("context broker registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identifier for the context broker that provides domain context").
			Security("non-sensitive").
			SystemUsage([]any{
				"context resolution",
				"relationship resolution",
			}).
			Validation("Must be a registered context broker").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z_]+$`).
			Required(true).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("EXT-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("spec_interpreter", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("routes to correct interpreter").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("spec interpreter registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Identifier for the spec interpreter that handles this domain").
			Security("non-sensitive").
			SystemUsage([]any{
				"spec loading",
				"validation",
				"querying",
			}).
			Validation("Must be a registered spec interpreter").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z_]+$`).
			Required(true).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("EXT-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ExtensibleObjectBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ExtensibleObjectBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ExtensibleObjectBuilder) GetOntology() string {
	return "extensible_object"
}

func init() {
	builders.RegisterBuilder(NewExtensibleObjectBuilder())
}
