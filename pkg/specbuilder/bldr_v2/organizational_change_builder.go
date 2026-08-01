package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// OrganizationalChangeBuilder builds the organizational_change spec at version v2_0_0
// File: bldr_v2/organizational_change_builder.go - version is encoded in package/directory name
type OrganizationalChangeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewOrganizationalChangeBuilder creates a new builder for organizational_change spec version v2_0_0
func NewOrganizationalChangeBuilder() *OrganizationalChangeBuilder {
	builder := &OrganizationalChangeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("organizational_change", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a specific organizational restructuring event such as division restructure, team reassignment, or organizational merge. Tracks the change event that can trigger impact analysis and change propagation. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addOrganizationalChangeFields()

	return builder
}

// addOrganizationalChangeFields adds the organizational_change fields
func (b *OrganizationalChangeBuilder) addOrganizationalChangeFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("affected_objects", "map").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for impact analysis and change propagation").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("organizational object registries").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Map of affected organizational objects by type (divisions, teams, organizations, etc.)").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"change propagation",
				"reporting",
			}).
			Validation("Map with keys representing object types and values being lists of object references").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("ORG-CHG-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_date", "datetime").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for chronological ordering and reporting").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Date/time when the organizational change occurred or is scheduled to occur").
			Security("non-sensitive").
			SystemUsage([]any{
				"chronological ordering",
				"reporting",
				"impact analysis",
			}).
			Validation("ISO-8601 datetime format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ORG-CHG-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for reporting and documentation").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable description of the organizational change (e.g., \\\"Split Engineering division into Infrastructure and Product divisions\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"reporting",
				"impact analysis",
			}).
			Validation("Free-form text describing the change").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ORG-CHG-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for categorization and filtering").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of organizational change (e.g., division_restructure, team_reassignment, organization_merge, team_move)").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
				"filtering",
				"reporting",
			}).
			Validation("Must be a non-empty string describing the change type").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ORG-CHG-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact_analysis_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for linking changes to impact analysis results").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("impact analysis registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to impact analysis objects generated for this organizational change").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"impact analysis",
				"reporting",
			}).
			Validation("Must reference existing impact_analysis object IDs").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("ORG-CHG-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *OrganizationalChangeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *OrganizationalChangeBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *OrganizationalChangeBuilder) GetOntology() string {
	return "organizational_change"
}

func init() {
	builders.RegisterBuilder(NewOrganizationalChangeBuilder())
}
