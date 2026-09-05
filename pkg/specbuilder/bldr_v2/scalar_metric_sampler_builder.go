package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ScalarMetricSamplerBuilder builds the scalar_metric_sampler spec at version v2_0_0
// File: bldr_v2/scalar_metric_sampler_builder.go - version is encoded in package/directory name
type ScalarMetricSamplerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewScalarMetricSamplerBuilder creates a new builder for scalar_metric_sampler spec version v2_0_0
func NewScalarMetricSamplerBuilder() *ScalarMetricSamplerBuilder {
	builder := &ScalarMetricSamplerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("scalar_metric_sampler", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_sampler").
		SetDescription("Sampler configuration for scalar metric types. Specialized sampler for single numeric values (count, duration, size) that can be aggregated using sum, average, min, max operations. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addScalarMetricSamplerFields()

	return builder
}

// addScalarMetricSamplerFields adds the scalar_metric_sampler fields
func (b *ScalarMetricSamplerBuilder) addScalarMetricSamplerFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("batch_size", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flush_interval", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("group_by_object_id", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_batch_size", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"scalar_metric",
			}).
			Required(true).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ScalarMetricSamplerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ScalarMetricSamplerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ScalarMetricSamplerBuilder) GetOntology() string {
	return "scalar_metric_sampler"
}

func init() {
	builders.RegisterBuilder(NewScalarMetricSamplerBuilder())
}
