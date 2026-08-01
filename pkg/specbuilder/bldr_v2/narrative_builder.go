package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// NarrativeBuilder builds the narrative spec at version v2_0_0
// File: bldr_v2/narrative_builder.go - version is encoded in package/directory name
type NarrativeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewNarrativeBuilder creates a new builder for narrative spec version v2_0_0
func NewNarrativeBuilder() *NarrativeBuilder {
	builder := &NarrativeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("narrative", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Models a narrative or story used for branding or marketing.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addNarrativeFields()

	return builder
}

// addNarrativeFields adds the narrative fields
func (b *NarrativeBuilder) addNarrativeFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("brand_asset_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("marketing manager.").
			AutomationHooks("asset inclusion.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("brand_asset registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Assets used in this narrative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"asset linking",
			}).
			Validation("list of asset references.").
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
		WithProfileCode("NAR-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("key_messages", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("marketing manager.").
			AutomationHooks("messaging.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Key messages conveyed by the narrative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"messaging",
			}).
			Validation("list of strings.").
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
		WithProfileCode("NAR-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("story_arc", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("marketing manager.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The structure or arc of the story.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
			}).
			Validation("string.").
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
		WithProfileCode("NAR-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_audience", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("marketing manager.").
			AutomationHooks("targeting.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target audience for this narrative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"targeting",
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
		WithProfileCode("NAR-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tone", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("marketing manager.").
			AutomationHooks("tone enforcement.").
			Cardinality("one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The tone of the narrative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tone alignment",
			}).
			Validation("string.").
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
		WithProfileCode("NAR-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *NarrativeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *NarrativeBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *NarrativeBuilder) GetOntology() string {
	return "narrative"
}

func init() {
	builders.RegisterBuilder(NewNarrativeBuilder())
}
