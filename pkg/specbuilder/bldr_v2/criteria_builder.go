package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CriteriaBuilder builds the criteria spec at version v2_0_0
// File: bldr_v2/criteria_builder.go - version is encoded in package/directory name
type CriteriaBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCriteriaBuilder creates a new builder for criteria spec version v2_0_0
func NewCriteriaBuilder() *CriteriaBuilder {
	builder := &CriteriaBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("criteria", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Reusable, independently-trackable validation conditions that can be linked to milestones, goals, backlog items, and other system objects. Enables sophisticated validation workflows with dependencies and different validation methods.\\nLifecycle: criteria_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addCriteriaFields()

	return builder
}

// addCriteriaFields adds the criteria fields
func (b *CriteriaBuilder) addCriteriaFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_item_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for backlog item validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items this criterion validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing backlog item IDs.").
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
		WithProfileCode("CRT-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner.").
			AutomationHooks("can be used for auto-tagging, filtering, and category-based reporting.").
			Cardinality("one").
			Criticality("association").
			Default("required - must be specified").
			Dependencies("criteria-categories-v1.0.md for category definitions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of criterion (functional, non-functional, acceptance, test, performance, security, compliance). See criteria-categories-v1.0.md for definitions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
				"traceability",
			}).
			Validation("enum (must be one of defined categories).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"functional",
				"non-functional",
				"acceptance",
				"test",
				"performance",
				"security",
				"compliance",
			}).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CRT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this criterion validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing goal IDs.").
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
		WithProfileCode("CRT-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone completion validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones this criterion validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing milestone IDs.").
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
		WithProfileCode("CRT-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for validation prioritization.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("validation workflow.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Priority level (critical, high, medium, low) for validation ordering.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"validation ordering",
			}).
			Validation("enum (critical, high, medium, low).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"critical",
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CRT-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for requirement validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Requirements this criterion validates.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"validation",
			}).
			Validation("must reference existing requirement IDs.").
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
		WithProfileCode("CRT-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("validator/owner.").
			AutomationHooks("triggers validation workflows.").
			Cardinality("one").
			Criticality("composition").
			Default("not_started").
			Dependencies("validation workflow.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Validation status of the criterion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle",
				"reporting",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"not_started",
				"in_progress",
				"validated",
				"complete",
				"blocked",
				"rejected",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CRT-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("validation_method", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner.").
			AutomationHooks("determines auto-transition behavior.").
			Cardinality("one").
			Criticality("composition").
			Default("manual_check").
			Dependencies("validation_threshold required for metric_threshold method.").
			Lifecycle("mutable (but changing may invalidate evidence).").
			Observability("yes").
			Purpose("How the criterion is validated (manual_check, automated_test, metric_threshold, code_review, external_approval).").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle",
				"automation",
				"reporting",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"manual_check",
				"automated_test",
				"metric_threshold",
				"code_review",
				"external_approval",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CRT-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("validation_threshold", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner.").
			AutomationHooks("used for automated validation checks.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("validation_method must be \"metric_threshold\".").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target value for metric-based validation (e.g., \\\\\\\"80%\\\\\\\", \\\\\\\"100ms\\\\\\\", \\\\\\\"0 errors\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"automation",
				"reporting",
			}).
			Validation("Free-form string describing threshold.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("CRT-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CriteriaBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CriteriaBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CriteriaBuilder) GetOntology() string {
	return "criteria"
}

func init() {
	builders.RegisterBuilder(NewCriteriaBuilder())
}
