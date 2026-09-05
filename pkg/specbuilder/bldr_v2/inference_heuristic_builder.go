package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// InferenceHeuristicBuilder builds the inference_heuristic spec at version v2_0_0
// File: bldr_v2/inference_heuristic_builder.go - version is encoded in package/directory name
type InferenceHeuristicBuilder struct {
	*builders.BaseSpecBuilder
}

// NewInferenceHeuristicBuilder creates a new builder for inference_heuristic spec version v2_0_0
func NewInferenceHeuristicBuilder() *InferenceHeuristicBuilder {
	builder := &InferenceHeuristicBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("inference_heuristic", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Inference Heuristic objects define dynamic rules for the InferenceEngine to suggest actionable backlog items based on maturity assessments.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addInferenceHeuristicFields()

	return builder
}

// addInferenceHeuristicFields adds the inference_heuristic fields
func (b *InferenceHeuristicBuilder) addInferenceHeuristicFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("condition_indicator_type", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("proposed_description", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("proposed_priority", "enum").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"P0",
				"P1",
				"P2",
				"P3",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("proposed_title", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_maturity_level", "integer").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *InferenceHeuristicBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *InferenceHeuristicBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *InferenceHeuristicBuilder) GetOntology() string {
	return "inference_heuristic"
}

func init() {
	builders.RegisterBuilder(NewInferenceHeuristicBuilder())
}
