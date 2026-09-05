package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// DigitalAssetBuilder builds the digital_asset spec at version v2_0_0
// File: bldr_v2/digital_asset_builder.go - version is encoded in package/directory name
type DigitalAssetBuilder struct {
	*builders.BaseSpecBuilder
}

// NewDigitalAssetBuilder creates a new builder for digital_asset spec version v2_0_0
func NewDigitalAssetBuilder() *DigitalAssetBuilder {
	builder := &DigitalAssetBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("digital_asset", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Models a digital asset including metadata, storage, quality, and lineage.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addDigitalAssetFields()

	return builder
}

// addDigitalAssetFields adds the digital_asset fields
func (b *DigitalAssetBuilder) addDigitalAssetFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("lineage", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("media manager.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Asset lineage.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DAS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("media manager.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Asset metadata.").
			Security("non-sensitive").
			SystemUsage([]any{
				"cataloging",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DAS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("quality_metrics", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("qa.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Quality metrics.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DAS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("storage", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Asset storage details.").
			Security("non-sensitive").
			SystemUsage([]any{
				"retrieval",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("DAS-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *DigitalAssetBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *DigitalAssetBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *DigitalAssetBuilder) GetOntology() string {
	return "digital_asset"
}

func init() {
	builders.RegisterBuilder(NewDigitalAssetBuilder())
}
