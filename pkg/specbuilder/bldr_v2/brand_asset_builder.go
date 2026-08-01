package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BrandAssetBuilder builds the brand_asset spec at version v2_0_0
// File: bldr_v2/brand_asset_builder.go - version is encoded in package/directory name
type BrandAssetBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBrandAssetBuilder creates a new builder for brand_asset spec version v2_0_0
func NewBrandAssetBuilder() *BrandAssetBuilder {
	builder := &BrandAssetBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("brand_asset", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Models a single brand asset (e.g., a logo, font, color palette, tagline, or media file).").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addBrandAssetFields()

	return builder
}

// addBrandAssetFields adds the brand_asset fields
func (b *BrandAssetBuilder) addBrandAssetFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("asset_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("brand manager.").
			AutomationHooks("categorization.").
			Cardinality("one").
			Criticality("core").
			Default("logo").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of the brand asset.").
			Security("non-sensitive").
			SystemUsage([]any{
				"asset categorization",
			}).
			Validation("valid enum value.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"logo",
				"font",
				"color_palette",
				"tagline",
				"media",
				"document",
				"template",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BRA-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("asset_url_or_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("brand manager.").
			AutomationHooks("asset fetching.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("URI or file path to the asset.").
			Security("non-sensitive").
			SystemUsage([]any{
				"asset locating",
			}).
			Validation("valid path or URI.").
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
		WithProfileCode("BRA-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("brand_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("brand manager.").
			AutomationHooks("brand association.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("brand registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the parent brand.").
			Security("non-sensitive").
			SystemUsage([]any{
				"brand association",
			}).
			Validation("must be a valid brand reference.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("BRA-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("usage_guidelines", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("brand manager.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Guidelines on how to use this asset.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
			}).
			Validation("free text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BRA-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BrandAssetBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BrandAssetBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BrandAssetBuilder) GetOntology() string {
	return "brand_asset"
}

func init() {
	builders.RegisterBuilder(NewBrandAssetBuilder())
}
