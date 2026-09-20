package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// NamespaceRegistryBuilder builds the namespace_registry spec at version v2_0_0
// File: bldr_v2/namespace_registry_builder.go - version is encoded in package/directory name
type NamespaceRegistryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewNamespaceRegistryBuilder creates a new builder for namespace_registry spec version v2_0_0
func NewNamespaceRegistryBuilder() *NamespaceRegistryBuilder {
	builder := &NamespaceRegistryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("namespace_registry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Registry that tracks all registered namespaces in the system. Enables namespace discovery, validation, and management.\\nLifecycle: namespace_registry_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("listable").
		AddTrait("readable").
		AddTrait("writable").
		AddTrait("modifiable").
		AddTrait("formatable").
		AddTrait("filterable").
		AddTrait("sortable").
		AddTrait("searchable")

	// Add fields
	builder.addNamespaceRegistryFields()

	return builder
}

// addNamespaceRegistryFields adds the namespace_registry fields
func (b *NamespaceRegistryBuilder) addNamespaceRegistryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^NAMESPACE-REGISTRY-\d{3,}$`).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("namespaces", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for namespace discovery").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("namespace objects").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of registered namespaces with their metadata.").
			Security("non-sensitive").
			SystemUsage([]any{
				"namespace_discovery",
				"validation",
			}).
			Validation("List of namespace registry entries").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(1).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("NSR-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *NamespaceRegistryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *NamespaceRegistryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *NamespaceRegistryBuilder) GetOntology() string {
	return "namespace_registry"
}

func init() {
	builders.RegisterBuilder(NewNamespaceRegistryBuilder())
}
