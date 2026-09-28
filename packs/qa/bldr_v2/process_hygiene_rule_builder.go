package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ProcessHygieneRuleBuilder builds the process_hygiene_rule spec at version v2_0_0
// File: bldr_v2/process_hygiene_rule_builder.go - version is encoded in package/directory name
type ProcessHygieneRuleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewProcessHygieneRuleBuilder creates a new builder for process_hygiene_rule spec version v2_0_0
func NewProcessHygieneRuleBuilder() *ProcessHygieneRuleBuilder {
	builder := &ProcessHygieneRuleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("process_hygiene_rule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Machine-oriented process hygiene rule row: declarative match on a single object field (prefix, suffix,\\nequals, or regex). Used by object-hygiene-scan when rules are loaded from object storage. Governance\\n`rule` objects remain separate human policy; this kind is operational configuration.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addProcessHygieneRuleFields()

	return builder
}

// addProcessHygieneRuleFields adds the process_hygiene_rule fields
func (b *ProcessHygieneRuleBuilder) addProcessHygieneRuleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			Cardinality("one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable explanation of the check.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Optional text.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation").
			Cardinality("one").
			Criticality("association").
			Default(true).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When false, rule is skipped.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
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
	b.AddFieldBuilder(builders.NewFieldBuilder("match_equals", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Match when field equals this literal.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Optional.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("match_field", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Object field name to read (e.g. title, id).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Non-empty when enabled.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("match_prefix", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Match when field value has this prefix (exclusive with suffix/equals/regex).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Optional.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("match_regex", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Match when field matches this regex (Go RE2 syntax).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Optional; must compile when set.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("match_suffix", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Match when field value has this suffix.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Optional.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("rule_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Stable identifier for findings (matches process hygiene rule id in YAML bundles).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Non-empty when enabled.").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("sort_order", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Ordering when multiple storage rules apply (lower first).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scan",
			}).
			Validation("Integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Short label for logs and UI.").
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
func (b *ProcessHygieneRuleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ProcessHygieneRuleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ProcessHygieneRuleBuilder) GetOntology() string {
	return "process_hygiene_rule"
}

func init() {
	builders.RegisterBuilder(NewProcessHygieneRuleBuilder())
}
