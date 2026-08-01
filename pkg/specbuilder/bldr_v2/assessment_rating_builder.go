package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AssessmentRatingBuilder builds the assessment_rating spec at version v2_0_0
// File: bldr_v2/assessment_rating_builder.go - version is encoded in package/directory name
type AssessmentRatingBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAssessmentRatingBuilder creates a new builder for assessment_rating spec version v2_0_0
func NewAssessmentRatingBuilder() *AssessmentRatingBuilder {
	builder := &AssessmentRatingBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("assessment_rating", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Tracks the performance rating and evolution of a persona or skill.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAssessmentRatingFields()

	return builder
}

// addAssessmentRatingFields adds the assessment_rating fields
func (b *AssessmentRatingBuilder) addAssessmentRatingFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("score", "float").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithSemanticType("count"))
	b.AddFieldBuilder(builders.NewFieldBuilder("skill_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithSemanticType("label"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AssessmentRatingBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AssessmentRatingBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AssessmentRatingBuilder) GetOntology() string {
	return "assessment_rating"
}

func init() {
	builders.RegisterBuilder(NewAssessmentRatingBuilder())
}
