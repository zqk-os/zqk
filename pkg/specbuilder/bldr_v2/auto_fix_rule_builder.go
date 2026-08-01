package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AutoFixRuleBuilder builds the auto_fix_rule spec at version v2_0_0
// File: bldr_v2/auto_fix_rule_builder.go - version is encoded in package/directory name
type AutoFixRuleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAutoFixRuleBuilder creates a new builder for auto_fix_rule spec version v2_0_0
func NewAutoFixRuleBuilder() *AutoFixRuleBuilder {
	builder := &AutoFixRuleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("auto_fix_rule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("System object that describes how to auto-fix certain validation issues by object kind and condition.\\nUsed to curate fix commands per (kind, category, tier, rule) so fixes stay consistent and editable\\nwithout code changes. Conditions are general (not per-object); one rule applies to many objects.\\nPlaceholders in fix_command_template: {object_id}, {kind}, {field}, {message}, {rule}, {tier}, {category}.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAutoFixRuleFields()

	return builder
}

// addAutoFixRuleFields adds the auto_fix_rule fields
func (b *AutoFixRuleBuilder) addAutoFixRuleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applies_to_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Object kind this rule applies to (e.g. backlog_item, requirement). Required.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Object kind identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Issue category to match (e.g. instance_validation, reference, integrity). Empty = any.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Category string or empty.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_message_contains", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Substring that issue message must contain. Empty = any.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Substring or empty.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_rule", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Validation rule to match (e.g. lifecycle, required). Empty = any.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Rule name or empty.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_tier", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(0).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Issue tier to match (1-4). 0 = any tier.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("0 or 1-4.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("one").
			Criticality("association").
			Default(true).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When false, rule is ignored. Default true.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("fix_command_template", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Fix command template. Placeholders: {object_id}, {kind}, {field}, {message}, {rule}, {tier}, {category}.\nExample: \\\"zqk object update {object_id} --field {field}=<VALUE>\\\"\nOr with query hint: \\\"zqk object update {object_id} --field milestone_refs+=<MILESTONE_ID:category=feature>\\\"\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"auto-fix execution",
			}).
			Validation("Template string with optional placeholders.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(100).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Lower value = higher priority when multiple rules match. Default 100.").
			Security("non-sensitive").
			SystemUsage([]any{
				"fix command lookup",
			}).
			Validation("Integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable label for this rule (e.g. \\\"Backlog item lifecycle milestone_refs\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"display",
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
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AutoFixRuleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AutoFixRuleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AutoFixRuleBuilder) GetOntology() string {
	return "auto_fix_rule"
}

func init() {
	builders.RegisterBuilder(NewAutoFixRuleBuilder())
}
