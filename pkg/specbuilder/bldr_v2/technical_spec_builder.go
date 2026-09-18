package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TechnicalSpecBuilder builds the technical_spec spec at version v2_0_0
// File: bldr_v2/technical_spec_builder.go - version is encoded in package/directory name
type TechnicalSpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTechnicalSpecBuilder creates a new builder for technical_spec spec version v2_0_0
func NewTechnicalSpecBuilder() *TechnicalSpecBuilder {
	builder := &TechnicalSpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("technical_spec", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Technical Specs define the engineering implementation architecture for specific domain problems, supporting the Context Onion by explicitly mapping down to Requirements.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addTechnicalSpecFields()

	return builder
}

// addTechnicalSpecFields adds the technical_spec fields
func (b *TechnicalSpecBuilder) addTechnicalSpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("component", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/owner.").
			AutomationHooks("groups tech specs by component.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target component or system module this spec covers.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("structured string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TSP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/owner.").
			AutomationHooks("used in prompt builders for domain context.").
			Cardinality("one").
			Criticality("association").
			Default("none").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Architectural explanation and context of the implementation approach.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"context layering",
			}).
			Validation("markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TSP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/owner.").
			AutomationHooks("maps technical specs to acceptance criteria.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default("required").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this technical specification addresses.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"context layering",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("TSP-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TechnicalSpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TechnicalSpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TechnicalSpecBuilder) GetOntology() string {
	return "technical_spec"
}

func init() {
	builders.RegisterBuilder(NewTechnicalSpecBuilder())
}
