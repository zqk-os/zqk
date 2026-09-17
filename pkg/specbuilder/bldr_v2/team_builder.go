package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TeamBuilder builds the team spec at version v2_0_0
// File: bldr_v2/team_builder.go - version is encoded in package/directory name
type TeamBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTeamBuilder creates a new builder for team spec version v2_0_0
func NewTeamBuilder() *TeamBuilder {
	builder := &TeamBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("team", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("extensible_object").
		SetDescription("Organizational team within a division or department. Teams contain members (accounts) and can be associated with zqk kernel workstreams. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addTeamFields()

	return builder
}

// addTeamFields adds the team fields
func (b *TeamBuilder) addTeamFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("department_ref", "reference").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for organizational hierarchy navigation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("department registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the department this team belongs to (optional if team belongs directly to division)").
			Security("non-sensitive").
			SystemUsage([]any{
				"organizational hierarchy",
				"navigation",
				"reporting",
			}).
			Validation("Must reference existing department object ID using domain:organizational:department:{id} format, or null").
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
		WithProfileCode("TEAM-003"))
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
			Purpose("Reference to the division this team belongs to (optional if team belongs to department)").
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
		WithProfileCode("TEAM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("member_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for team membership tracking and permissions").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("account registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to accounts that are members of this team").
			Security("may contain PII—treat as confidential").
			SystemUsage([]any{
				"team membership",
				"permissions",
				"reporting",
			}).
			Validation("Must reference existing account object IDs using account:{id} format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("TEAM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("team_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for display and identification").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The name of the team").
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
		WithProfileCode("TEAM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for workstream-team relationship tracking").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to zqk kernel workstreams that this team is working on").
			Security("non-sensitive").
			SystemUsage([]any{
				"workstream tracking",
				"team workload",
			}).
			Validation("Must reference existing workstream object IDs using zqk:kernel:workstream:{id} format").
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
		WithProfileCode("TEAM-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TeamBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TeamBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TeamBuilder) GetOntology() string {
	return "team"
}

func init() {
	builders.RegisterBuilder(NewTeamBuilder())
}
