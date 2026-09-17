package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// StrategicContextBuilder builds the strategic_context spec at version v2_0_0
// File: bldr_v2/strategic_context_builder.go - version is encoded in package/directory name
type StrategicContextBuilder struct {
	*builders.BaseSpecBuilder
}

// NewStrategicContextBuilder creates a new builder for strategic_context spec version v2_0_0
func NewStrategicContextBuilder() *StrategicContextBuilder {
	builder := &StrategicContextBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("strategic_context", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Strategic context captures strategic information beyond policies (market conditions, important dates, goals).\\nUsed by alignment and policy generation. See project-discovery-and-strategic-alignment-v1.0.md.\\nLifecycle: strategic_context_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addStrategicContextFields()

	return builder
}

// addStrategicContextFields adds the strategic_context fields
func (b *StrategicContextBuilder) addStrategicContextFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("content", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal discovery and reports.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Multiline description of the strategic context.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"discovery",
			}).
			Validation("Free-form text.").
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
		WithProfileCode("SC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for alignment and filtering.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of context (e.g. market_condition, technical_constraint, business_constraint).").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"reporting",
			}).
			Validation("Free-form string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable", "listable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal alignment (affects_goals in doc).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this context affects (alias affects_goals in design).").
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
		WithProfileCode("SC-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("important_dates", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for timeline alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of {date, event, impact} entries.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
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
		WithProfileCode("SC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for policy-context alignment (affects_policies in doc).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Policies this context affects.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
			}).
			Validation("Must reference existing policy IDs.").
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
		WithProfileCode("SC-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stakeholder_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for stakeholder-work alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("stakeholder_profile or account registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to stakeholders (e.g. STK-001, account:executive-team).").
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
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("SC-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *StrategicContextBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *StrategicContextBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *StrategicContextBuilder) GetOntology() string {
	return "strategic_context"
}

func init() {
	builders.RegisterBuilder(NewStrategicContextBuilder())
}
