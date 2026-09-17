package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// OrganizationBuilder builds the organization spec at version v2_0_0
// File: bldr_v2/organization_builder.go - version is encoded in package/directory name
type OrganizationBuilder struct {
	*builders.BaseSpecBuilder
}

// NewOrganizationBuilder creates a new builder for organization spec version v2_0_0
func NewOrganizationBuilder() *OrganizationBuilder {
	builder := &OrganizationBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("organization", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Organizational entity representing a company, institution, or other organizational structure. Organizations can contain divisions and form partnerships with other organizations. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addOrganizationFields()

	return builder
}

// addOrganizationFields adds the organization fields
func (b *OrganizationBuilder) addOrganizationFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("division_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for organizational hierarchy navigation").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("division registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to divisions that belong to this organization").
			Security("non-sensitive").
			SystemUsage([]any{
				"organizational hierarchy",
				"navigation",
				"reporting",
			}).
			Validation("Must reference existing division object IDs using domain:organizational:division:{id} format").
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
		WithProfileCode("ORG-002"))
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
			Purpose("References to kernel goals that align with this organization").
			Security("non-sensitive").
			SystemUsage([]any{
				"strategic alignment",
				"goal tracking",
			}).
			Validation("Must reference existing goal object IDs using ZQK:kernel:goal:{id} format").
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
		WithProfileCode("ORG-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("organization_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for display and identification").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The official name of the organization").
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
		WithProfileCode("ORG-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *OrganizationBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *OrganizationBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *OrganizationBuilder) GetOntology() string {
	return "organization"
}

func init() {
	builders.RegisterBuilder(NewOrganizationBuilder())
}
