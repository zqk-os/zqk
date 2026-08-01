package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RequirementBuilder builds the requirement spec at version v2_0_0
// File: bldr_v2/requirement_builder.go - version is encoded in package/directory name
type RequirementBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRequirementBuilder creates a new builder for requirement spec version v2_0_0
func NewRequirementBuilder() *RequirementBuilder {
	builder := &RequirementBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("requirement", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Requirements describe specific deliverables linked to goals, with acceptance criteria and dependencies.\\nLifecycle: requirement_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRequirementFields()

	return builder
}

// addRequirementFields adds the requirement fields
func (b *RequirementBuilder) addRequirementFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("acceptance_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/reviewer.").
			AutomationHooks("maps to tests.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("testing frameworks.").
			Lifecycle("mutable (append/edit via review).").
			Observability("yes").
			Purpose("Legacy free-form text acceptance criteria (deprecated - use criteria_refs instead).").
			Security("non-sensitive").
			SystemUsage([]any{
				"testing",
				"reviews",
			}).
			Validation("structured text; encourage Given/When/Then style.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(0).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("REQ-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("actual_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("used for effort variance analysis.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("time tracking.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Actual effort spent on completing this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
			}).
			Validation("free text or structured format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REQ-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("updates backlog item progress.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items that implement this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"planning",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("completed_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("automatically set when status changes to complete.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("lifecycle transitions.").
			Lifecycle("mutable (set when status transitions to complete).").
			Observability("yes").
			Purpose("ISO-8601 datetime when the requirement was completed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
			}).
			Validation("ISO-8601 datetime format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REQ-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for completion validation.").
			Cardinality("many (>=1)").
			Criticality("composition").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Criteria objects that define requirement completion. Requirements MUST reference at least one criteria object (CRIT-####) instead of using free-form acceptance_criteria strings.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"completion tracking",
			}).
			Validation("must reference existing criteria object IDs (CRIT-#### format).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(0).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports, requirement documentation.").
			Cardinality("one").
			Criticality("association").
			Default("none (optional)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Detailed description of the requirement, its context, and expected outcomes.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"communication",
				"planning",
			}).
			Validation("free text or markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REQ-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("estimated_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for capacity planning.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("planning tools.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Estimated effort for completing this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"scheduling",
			}).
			Validation("free text or structured format.").
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
		WithProfileCode("REQ-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("updates goal progress.").
			Cardinality("one_or_more").
			Criticality("composition").
			Default("required").
			Dependencies("dashboards.").
			Lifecycle("mutable (with approvals).").
			Observability("yes").
			Purpose("Goals fulfilled by this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(0).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("updates milestone progress.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones that include this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"planning",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("influences scheduling.").
			Cardinality("one").
			Criticality("composition").
			Default("p2").
			Dependencies("scheduling.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Relative urgency (p0..p3 or similar).").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"p0",
				"p1",
				"p2",
				"p3",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("REQ-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("technical_spec_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/owner.").
			AutomationHooks("traces requirement back to technical architecture spec.").
			Cardinality("zero_or_more").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Technical specifications detailing the implementation approach for this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"context layering",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("test_case_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/QA.").
			AutomationHooks("updates test coverage metrics.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("test registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Test cases that validate this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"test coverage",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("updates workstream progress.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams implementing this requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"planning",
			}).
			Validation("IDs exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("REQ-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RequirementBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RequirementBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RequirementBuilder) GetOntology() string {
	return "requirement"
}

func init() {
	builders.RegisterBuilder(NewRequirementBuilder())
}
