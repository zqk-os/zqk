package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// NamespaceBuilder builds the namespace spec at version v2_0_0
// File: bldr_v2/namespace_builder.go - version is encoded in package/directory name
type NamespaceBuilder struct {
	*builders.BaseSpecBuilder
}

// NewNamespaceBuilder creates a new builder for namespace spec version v2_0_0
func NewNamespaceBuilder() *NamespaceBuilder {
	builder := &NamespaceBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("namespace", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a namespace that groups related object types and provides a naming scope. Namespaces enable categorization, isolation, integration, and discovery of object types.\\nLifecycle: namespace_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addNamespaceFields()

	return builder
}

// addNamespaceFields adds the namespace fields
func (b *NamespaceBuilder) addNamespaceFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applicability", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for applicability checks").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Defines when, where, and how the namespace can be used.").
			Security("non-sensitive").
			SystemUsage([]any{
				"usage_guidance",
				"validation",
			}).
			Validation("Namespace applicability object structure").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("NS-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("domain", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for domain-based queries").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("layer must be domain or integration").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Domain name for domain and integration layers (e.g., \\\\\\\"organizational\\\\\\\", \\\\\\\"financial\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
				"discovery",
			}).
			Validation("Domain name pattern").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9_]+$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("NS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("integration", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for cross-namespace validation").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Defines how namespaces integrate with each other while maintaining boundaries.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reference_validation",
				"integration_validation",
			}).
			Validation("Namespace integration object structure").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("NS-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("isolation", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for conflict detection and resolution").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Defines mechanisms to keep namespaces separate and prevent conflicts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"conflict_prevention",
				"access_control",
			}).
			Validation("Namespace isolation object structure").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("NS-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("layer", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("determines isolation rules").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Namespace layer (kernel, domain, or integration).").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
				"isolation",
			}).
			Validation("Must be one of kernel, domain, or integration").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"kernel",
				"domain",
				"integration",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("NS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("namespace_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for reference resolution").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("namespace_registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Unique namespace identifier (e.g., \\\\\\\"zqk:kernel\\\\\\\", \\\\\\\"domain:organizational\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"reference_resolution",
				"namespace_discovery",
			}).
			Validation("Namespace ID format pattern").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("NS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("origin", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("namespace_owner").
			AutomationHooks("used for authority checks").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Describes where the namespace comes from and who has authority over it.").
			Security("non-sensitive").
			SystemUsage([]any{
				"authority_validation",
				"discovery",
			}).
			Validation("Namespace origin object structure").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("NS-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *NamespaceBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *NamespaceBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *NamespaceBuilder) GetOntology() string {
	return "namespace"
}

func init() {
	builders.RegisterBuilder(NewNamespaceBuilder())
}
