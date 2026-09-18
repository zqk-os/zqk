package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ImportantDateBuilder builds the important_date spec at version v2_0_0
// File: bldr_v2/important_date_builder.go - version is encoded in package/directory name
type ImportantDateBuilder struct {
	*builders.BaseSpecBuilder
}

// NewImportantDateBuilder creates a new builder for important_date spec version v2_0_0
func NewImportantDateBuilder() *ImportantDateBuilder {
	builder := &ImportantDateBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("important_date", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Important date tracks critical dates and their impact (deadlines, milestones, market windows).\\nUsed by alignment and context validation. See project-discovery-and-strategic-alignment-v1.0.md.\\nLifecycle: important_date_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addImportantDateFields()

	return builder
}

// addImportantDateFields adds the important_date fields
func (b *ImportantDateBuilder) addImportantDateFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for timeline alignment and reminders.").
			Cardinality("one").
			Criticality("composition").
			Default("required at creation").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 date (e.g. 2026-03-31).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"reminders",
			}).
			Validation("ISO-8601 date format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable", "listable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DATE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("date_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for filtering (deadline vs milestone vs window).").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of date (e.g. deadline, milestone, market_window).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Free-form string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DATE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dependencies", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for dependency analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Free-form dependency descriptions (e.g. MIL-010 must complete by 2026-03-15).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("List of strings.").
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
		WithProfileCode("DATE-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-date alignment (related_goals in doc).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals related to this date.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Must reference existing goal IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DATE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact_scope", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for scope filtering.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scope of impact (e.g. project_wide, team, release).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Free-form string.").
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
		WithProfileCode("DATE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("importance", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for prioritization in reports.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Importance level (e.g. critical, high, medium).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Free-form string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DATE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone-date alignment (related_milestones in doc).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones related to this date.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Must reference existing milestone IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("DATE-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stakeholder_notifications", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for reminder scheduling.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of {stakeholder, notification_days_before} for reminders.").
			Security("non-sensitive").
			SystemUsage([]any{
				"notifications",
			}).
			Validation("List of objects.").
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
		WithProfileCode("DATE-008"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ImportantDateBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ImportantDateBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ImportantDateBuilder) GetOntology() string {
	return "important_date"
}

func init() {
	builders.RegisterBuilder(NewImportantDateBuilder())
}
