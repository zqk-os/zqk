package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// RuleBuilder builds the rule spec at version v2_0_0
// File: bldr_v2/rule_builder.go - version is encoded in package/directory name
type RuleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewRuleBuilder creates a new builder for rule spec version v2_0_0
func NewRuleBuilder() *RuleBuilder {
	builder := &RuleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("rule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Governance rules describing constraints, recommendations, and enforcement. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addRuleFields()

	return builder
}

// addRuleFields adds the rule fields
func (b *RuleBuilder) addRuleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("body", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used to generate enforcement hints.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("enforcement hooks.").
			Lifecycle("mutable (with history).").
			Observability("yes").
			Purpose("The actual rule instructions/policy.").
			Security("may contain sensitive workflows.").
			SystemUsage([]any{
				"docs",
				"enforcement",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RUL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-rule traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this rule supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing goal IDs.").
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
		WithProfileCode("RUL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scope", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("determines where the rule is enforced.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("enforcement hooks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Domain the rule applies to (command_execution, documentation, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"enforcement",
			}).
			Validation("controlled vocab.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RUL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("determines enforcement strictness.").
			Cardinality("one").
			Criticality("composition").
			Default("recommendation").
			Dependencies("enforcement hooks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Severity (\\\\\\\"hard_limit\\\\\\\", \\\\\\\"required\\\\\\\", \\\\\\\"recommendation\\\\\\\", \\\\\\\"informational\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"enforcement",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"hard_limit",
				"required",
				"recommendation",
				"informational",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("RUL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for workstream-specific enforcement.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this rule applies to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"enforcement",
				"filtering",
			}).
			Validation("Must reference existing workstream IDs.").
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
		WithProfileCode("RUL-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *RuleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *RuleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *RuleBuilder) GetOntology() string {
	return "rule"
}

func init() {
	builders.RegisterBuilder(NewRuleBuilder())
}
