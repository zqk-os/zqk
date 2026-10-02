package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ListMetricSamplerBuilder builds the list_metric_sampler spec at version v2_0_0
// File: bldr_v2/list_metric_sampler_builder.go - version is encoded in package/directory name
type ListMetricSamplerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewListMetricSamplerBuilder creates a new builder for list_metric_sampler spec version v2_0_0
func NewListMetricSamplerBuilder() *ListMetricSamplerBuilder {
	builder := &ListMetricSamplerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("list_metric_sampler", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_sampler").
		SetDescription("Sampler configuration for list metric types. Specialized sampler for unordered collections (tags, categories) that can be aggregated by value frequency, distribution, and unique count. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addListMetricSamplerFields()

	return builder
}

// addListMetricSamplerFields adds the list_metric_sampler fields
func (b *ListMetricSamplerBuilder) addListMetricSamplerFields() {
	addSamplerBatchAndIntervalFields(b.BaseSpecBuilder)
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"list_metric",
			}).
			Required(true).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ListMetricSamplerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ListMetricSamplerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ListMetricSamplerBuilder) GetOntology() string {
	return "list_metric_sampler"
}

func init() {
	builders.RegisterBuilder(NewListMetricSamplerBuilder())
}
