package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ImpactAnalysisBuilder builds the impact_analysis spec at version v2_0_0
// File: bldr_v2/impact_analysis_builder.go - version is encoded in package/directory name
type ImpactAnalysisBuilder struct {
	*builders.BaseSpecBuilder
}

// NewImpactAnalysisBuilder creates a new builder for impact_analysis spec version v2_0_0
func NewImpactAnalysisBuilder() *ImpactAnalysisBuilder {
	builder := &ImpactAnalysisBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("impact_analysis", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Stores impact analysis results including affected objects, impact severity, and recommended actions. Used to analyze and document the effects of organizational changes, schema changes, or other system modifications. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addImpactAnalysisFields()

	return builder
}

// addImpactAnalysisFields adds the impact_analysis fields
func (b *ImpactAnalysisBuilder) addImpactAnalysisFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("affected_objects", "map").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for impact propagation and reporting").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("object registries").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Map of affected kernel objects organized by type (workstreams, goals, backlog_items, milestones, etc.)").
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
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("IMP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to link analysis to source change").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("change tracking system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the change object that triggered this impact analysis").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"change tracking",
				"impact analysis",
			}).
			Validation("Must reference an existing change object ID").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("IMP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("change_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for categorization and filtering").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of change that triggered this analysis (e.g., division_restructure, schema_change, organizational_change)").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
				"filtering",
				"reporting",
			}).
			Validation("Free-form string describing the change type").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("IMP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact_categories", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for impact categorization and severity analysis").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of impact categories with severity, affected count, and descriptions").
			Security("non-sensitive").
			SystemUsage([]any{
				"impact analysis",
				"severity tracking",
				"reporting",
			}).
			Validation("List of objects with category, severity, affected_count, and description fields").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("IMP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("recommended_actions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for automated change propagation and workflow").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of recommended actions to address the impacts, including action type, priority, and affected objects").
			Security("non-sensitive").
			SystemUsage([]any{
				"change propagation",
				"workflow automation",
				"reporting",
			}).
			Validation("List of objects with action, priority, and affected_objects fields").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("IMP-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ImpactAnalysisBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ImpactAnalysisBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ImpactAnalysisBuilder) GetOntology() string {
	return "impact_analysis"
}

func init() {
	builders.RegisterBuilder(NewImpactAnalysisBuilder())
}
