package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// DecisionBuilder builds the decision spec at version v2_0_0
// File: bldr_v2/decision_builder.go - version is encoded in package/directory name
type DecisionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDecisionBuilder creates a new builder for decision spec version v2_0_0
func NewDecisionBuilder() *DecisionBuilder {
	builder := &DecisionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("decision", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Decisions capture governance choices, linking goals, requirements, rules, and workstreams. Architecture Decision Records (ADRs) use the ADR-XXX format, while other decisions use DEC-XXX.\\nLifecycle: decision_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addDecisionFields()

	return builder
}

// addDecisionFields adds the decision fields
func (b *DecisionBuilder) addDecisionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("decision_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for decision dependency graphs.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("decision registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to related or dependent decisions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"dependency tracking",
			}).
			Validation("must reference existing decision IDs.").
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
		WithProfileCode("DEC-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for goal impact analysis.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals impacted by this decision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("must reference existing goal IDs.").
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
		WithProfileCode("DEC-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per kind sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier for decision objects (DEC-### or ADR-### format). ADR-### format is used for Architecture Decision Records.").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^(DEC|ADR)-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(DEC|ADR)-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("DEC-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive").
			AutomationHooks("used for release notes.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("goal/requirement summaries.").
			Lifecycle("mutable (through revisions)").
			Observability("yes").
			Purpose("Summarize downstream effects/outcomes of the decision.").
			Security("may contain sensitive discussion; treat as confidential.").
			SystemUsage([]any{
				"reporting",
				"goal dashboards",
			}).
			Validation("markdown allowed.").
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
		WithProfileCode("DEC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact_level", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("highlight critical decisions.").
			Cardinality("one").
			Criticality("association").
			Default("primary").
			Dependencies("goal dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Strength of goal linkage (\\\\\\\"primary\\\\\\\", \\\\\\\"secondary\\\\\\\", \\\\\\\"observed\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"goal dashboards",
			}).
			Validation("enum (\\\\\\\"primary\\\\\\\", \\\\\\\"secondary\\\\\\\", \\\\\\\"observed\\\\\\\").").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"primary",
				"secondary",
				"observed",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DEC-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for milestone impact analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones impacted by this decision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("must reference existing milestone IDs.").
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
		WithProfileCode("DEC-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for requirement impact analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements impacted by this decision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("must reference existing requirement IDs.").
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
		WithProfileCode("DEC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("revisit", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("schedule follow-up tasks.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler triggers.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When to revisit/reassess the decision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
				"reminders",
			}).
			Validation("ISO-8601.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DEC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for workstream impact analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams impacted by this decision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("must reference existing workstream IDs.").
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
		WithProfileCode("DEC-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DecisionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DecisionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DecisionBuilder) GetOntology() string {
	return "decision"
}

func init() {
	builders.RegisterBuilder(NewDecisionBuilder())
}
