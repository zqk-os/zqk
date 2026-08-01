package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CommercialSequenceBuilder builds the commercial_sequence spec at version v2_0_0
// File: bldr_v2/commercial_sequence_builder.go - version is encoded in package/directory name
type CommercialSequenceBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCommercialSequenceBuilder creates a new builder for commercial_sequence spec version v2_0_0
func NewCommercialSequenceBuilder() *CommercialSequenceBuilder {
	builder := &CommercialSequenceBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("commercial_sequence", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Models a sequence of commercial interactions or steps (e.g., a sales funnel, email campaign, or user onboarding sequence).").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addCommercialSequenceFields()

	return builder
}

// addCommercialSequenceFields adds the commercial_sequence fields
func (b *CommercialSequenceBuilder) addCommercialSequenceFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("conversion_goal", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("sales/marketing manager.").
			AutomationHooks("analytics.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The ultimate goal of the sequence.").
			Security("non-sensitive").
			SystemUsage([]any{
				"measurement",
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
		WithProfileCode("CSQ-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("narrative_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("sales/marketing manager.").
			AutomationHooks("narrative inclusion.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("narrative registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Narratives employed in this sequence.").
			Security("non-sensitive").
			SystemUsage([]any{
				"content linking",
			}).
			Validation("list of narrative references.").
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
		WithProfileCode("CSQ-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sequence_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("sales/marketing manager.").
			AutomationHooks("workflow triggering.").
			Cardinality("one").
			Criticality("core").
			Default("sales_funnel").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The type of sequence.").
			Security("non-sensitive").
			SystemUsage([]any{
				"categorization",
			}).
			Validation("enum string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"sales_funnel",
				"email_campaign",
				"onboarding",
				"advertisement",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CSQ-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stages", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("sales/marketing manager.").
			AutomationHooks("stage tracking.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The steps or stages in the sequence.").
			Security("non-sensitive").
			SystemUsage([]any{
				"progression",
			}).
			Validation("list of strings.").
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
		WithProfileCode("CSQ-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CommercialSequenceBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CommercialSequenceBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CommercialSequenceBuilder) GetOntology() string {
	return "commercial_sequence"
}

func init() {
	builders.RegisterBuilder(NewCommercialSequenceBuilder())
}
