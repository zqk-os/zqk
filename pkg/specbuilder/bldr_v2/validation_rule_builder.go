package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ValidationRuleBuilder builds the validation_rule spec at version v2_0_0
// File: bldr_v2/validation_rule_builder.go - version is encoded in package/directory name
type ValidationRuleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewValidationRuleBuilder creates a new builder for validation_rule spec version v2_0_0
func NewValidationRuleBuilder() *ValidationRuleBuilder {
	builder := &ValidationRuleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("validation_rule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a programmable validation rule that enforces constraints during object lifecycle transitions.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addValidationRuleFields()

	return builder
}

// addValidationRuleFields adds the validation_rule fields
func (b *ValidationRuleBuilder) addValidationRuleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("parameters", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("one").
			Criticality("composition").
			Observability("yes").
			Purpose("Configuration parameters mapping values for the rule_type.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("rule_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("one").
			Criticality("composition").
			Observability("yes").
			Purpose("The generic validator template (field_presence, active_reference, alignment, expression).").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"field_presence",
				"active_reference",
				"alignment",
				"expression",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sequence_level", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("one").
			Criticality("composition").
			Observability("yes").
			Purpose("Evaluation tier (1 = local completeness, 2 = dereference/active checks, 3 = path integrity).").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("one").
			Criticality("composition").
			Observability("yes").
			Purpose("The object kind this rule restricts (e.g. backlog_item, milestone).").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_status", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("one").
			Criticality("composition").
			Observability("yes").
			Purpose("The target lifecycle status (e.g. active, complete, in_progress) this rule restricts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("transition_from", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Observability("yes").
			Purpose("Optional source status limiting this rule to a specific transition path.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ValidationRuleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ValidationRuleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ValidationRuleBuilder) GetOntology() string {
	return "validation_rule"
}

func init() {
	builders.RegisterBuilder(NewValidationRuleBuilder())
}
