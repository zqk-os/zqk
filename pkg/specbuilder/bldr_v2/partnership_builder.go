package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// PartnershipBuilder builds the partnership spec at version v2_0_0
// File: bldr_v2/partnership_builder.go - version is encoded in package/directory name
type PartnershipBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPartnershipBuilder creates a new builder for partnership spec version v2_0_0
func NewPartnershipBuilder() *PartnershipBuilder {
	builder := &PartnershipBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("partnership", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Partnership relationship between two or more organizations. Partnerships represent collaborative relationships, joint ventures, or other inter-organizational arrangements. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPartnershipFields()

	return builder
}

// addPartnershipFields adds the partnership fields
func (b *PartnershipBuilder) addPartnershipFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("end_date", "date").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for partnership timeline tracking").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The date when the partnership ended (null for active partnerships)").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeline tracking",
				"reporting",
			}).
			Validation("Must be a valid ISO 8601 date, must be after start_date if both are provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("date").
		WithProfileCode("PART-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("organization_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for partnership relationship tracking").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("organization registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to organizations participating in this partnership (minimum 2)").
			Security("non-sensitive").
			SystemUsage([]any{
				"partnership tracking",
				"relationship analysis",
			}).
			Validation("Must reference at least 2 existing organization object IDs using domain:organizational:organization:{id} format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PART-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("partnership_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for display and identification").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The name or description of the partnership").
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
		WithTraits("readable", "writable", "modifiable", "searchable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PART-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("partnership_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for partnership categorization and filtering").
			Cardinality("one").
			Criticality("association").
			Default("collaboration").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The type of partnership (collaboration, joint_venture, strategic_alliance, etc.)").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
				"filtering",
				"reporting",
			}).
			Validation("Must be one of the defined partnership types").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"collaboration",
				"joint_venture",
				"strategic_alliance",
				"supplier",
				"customer",
				"other",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PART-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("start_date", "date").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for partnership timeline tracking").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The date when the partnership began").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeline tracking",
				"reporting",
			}).
			Validation("Must be a valid ISO 8601 date").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("date").
		WithProfileCode("PART-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PartnershipBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PartnershipBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PartnershipBuilder) GetOntology() string {
	return "partnership"
}

func init() {
	builders.RegisterBuilder(NewPartnershipBuilder())
}
