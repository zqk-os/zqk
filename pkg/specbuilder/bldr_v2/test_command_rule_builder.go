package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TestCommandRuleBuilder builds the test_command_rule spec at version v2_0_0
// File: bldr_v2/test_command_rule_builder.go - version is encoded in package/directory name
type TestCommandRuleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTestCommandRuleBuilder creates a new builder for test_command_rule spec version v2_0_0
func NewTestCommandRuleBuilder() *TestCommandRuleBuilder {
	builder := &TestCommandRuleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("test_command_rule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Rule that defines when a command line (command + args) is treated as a \\\"test command\\\"\\nfor scheduler run_wrapper jobs. Used for notification behavior (e.g. test-failure vs generic failure)\\nand output sanitization. Rules are evaluated in order; first match wins.\\nEach rule has conditions (field/operator/value). Supported fields: command, args, shell_script.\\nOperators: eq, contains, in. See scheduler test command detection docs.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addTestCommandRuleFields()

	return builder
}

// addTestCommandRuleFields adds the test_command_rule fields
func (b *TestCommandRuleBuilder) addTestCommandRuleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("conditions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("consumed by scheduler run_wrapper to detect test commands.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of conditions; all must match for this rule to match. Each item has field (command|args|shell_script), operator (eq|contains|in), value (string or array).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("list of objects with field, operator, value.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TCR-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional description of what this rule matches.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TCR-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Short name for this rule (for display/admin).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TCR-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines evaluation order (lower number = higher priority, evaluated first).").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Evaluation order; lower value = higher priority (e.g. 0 before 10).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("TCR-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TestCommandRuleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TestCommandRuleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TestCommandRuleBuilder) GetOntology() string {
	return "test_command_rule"
}

func init() {
	builders.RegisterBuilder(NewTestCommandRuleBuilder())
}
