package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// DomainRegistryBuilder builds the domain_registry spec at version v2_0_0
// File: bldr_v2/domain_registry_builder.go - version is encoded in package/directory name
type DomainRegistryBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDomainRegistryBuilder creates a new builder for domain_registry spec version v2_0_0
func NewDomainRegistryBuilder() *DomainRegistryBuilder {
	builder := &DomainRegistryBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("domain_registry", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Registry that tracks all registered domain ontologies in the system. Enables domain discovery, validation, and integration of external domain models.\\nLifecycle: domain_registry_lifecycle.yaml.\\n").
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
	builder.addDomainRegistryFields()

	return builder
}

// addDomainRegistryFields adds the domain_registry fields
func (b *DomainRegistryBuilder) addDomainRegistryFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("domains", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("used for domain discovery").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("domain namespace objects").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of registered domain ontologies with their metadata.").
			Security("non-sensitive").
			SystemUsage([]any{
				"domain_discovery",
				"validation",
				"integration",
			}).
			Validation("List of domain registry entries").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinCount(0).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("DMR-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^DOMAIN-REG-\d{3,}$`).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DomainRegistryBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DomainRegistryBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DomainRegistryBuilder) GetOntology() string {
	return "domain_registry"
}

func init() {
	builders.RegisterBuilder(NewDomainRegistryBuilder())
}
