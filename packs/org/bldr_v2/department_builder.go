package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// DepartmentBuilder builds the department spec at version v2_0_0
// File: bldr_v2/department_builder.go - version is encoded in package/directory name
type DepartmentBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDepartmentBuilder creates a new builder for department spec version v2_0_0
func NewDepartmentBuilder() *DepartmentBuilder {
	builder := &DepartmentBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("department", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Organizational department within a division or organization. Departments are typically smaller organizational units than divisions and can contain teams. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addDepartmentFields()

	return builder
}

// addDepartmentFields adds the department fields
func (b *DepartmentBuilder) addDepartmentFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("department_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for display and identification").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The name of the department").
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
		WithProfileCode("DEPT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("division_ref", "reference").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for organizational hierarchy navigation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("division registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the division this department belongs to (optional if department is directly under organization)").
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
		WithProfileCode("DEPT-002"))
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
			Purpose("References to kernel goals that align with this department").
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
		WithProfileCode("DEPT-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DepartmentBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DepartmentBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DepartmentBuilder) GetOntology() string {
	return "department"
}

func init() {
	builders.RegisterBuilder(NewDepartmentBuilder())
}
