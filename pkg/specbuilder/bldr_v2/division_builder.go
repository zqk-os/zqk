package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// DivisionBuilder builds the division spec at version v2_0_0
// File: bldr_v2/division_builder.go - version is encoded in package/directory name
type DivisionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDivisionBuilder creates a new builder for division spec version v2_0_0
func NewDivisionBuilder() *DivisionBuilder {
	builder := &DivisionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("division", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Organizational division within an organization. Divisions can be hierarchical (parent/child) and contain teams. Divisions can align with zqk kernel goals. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addDivisionFields()

	return builder
}

// addDivisionFields adds the division fields
func (b *DivisionBuilder) addDivisionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("division_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for display and identification").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The name of the division").
			Security("non-sensitive").
			SystemUsage([]any{
				"display",
				"identification",
				"search",
			}).
			Validation("Must be a non-empty string").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DIV-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("kernel_goals_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for strategic alignment tracking").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to kernel goals that align with this division").
			Security("non-sensitive").
			SystemUsage([]any{
				"strategic alignment",
				"goal tracking",
			}).
			Validation("Must reference existing goal object IDs using zqk:kernel:goal:{id} format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("DIV-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("parent_division_ref", "reference").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for hierarchical navigation and reporting").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("division registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the parent division (null for top-level divisions)").
			Security("non-sensitive").
			SystemUsage([]any{
				"organizational hierarchy",
				"navigation",
				"reporting",
			}).
			Validation("Must reference existing division object ID using domain:organizational:division:{id} format, or null").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("DIV-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DivisionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DivisionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DivisionBuilder) GetOntology() string {
	return "division"
}

func init() {
	builders.RegisterBuilder(NewDivisionBuilder())
}
