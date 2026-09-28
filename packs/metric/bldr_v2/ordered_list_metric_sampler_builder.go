package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// OrderedListMetricSamplerBuilder builds the ordered_list_metric_sampler spec at version v2_0_0
// File: bldr_v2/ordered_list_metric_sampler_builder.go - version is encoded in package/directory name
type OrderedListMetricSamplerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewOrderedListMetricSamplerBuilder creates a new builder for ordered_list_metric_sampler spec version v2_0_0
func NewOrderedListMetricSamplerBuilder() *OrderedListMetricSamplerBuilder {
	builder := &OrderedListMetricSamplerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("ordered_list_metric_sampler", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_sampler").
		SetDescription("Sampler configuration for ordered list metric types. Specialized sampler for ordered sequences (event sequences, time-series) where order matters. Can be analyzed for patterns, trends, and sequences. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addOrderedListMetricSamplerFields()

	return builder
}

// addOrderedListMetricSamplerFields adds the ordered_list_metric_sampler fields
func (b *OrderedListMetricSamplerBuilder) addOrderedListMetricSamplerFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("batch_size", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("flush_interval", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("group_by_object_id", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_batch_size", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"ordered_list_metric",
			}).
			Required(true).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *OrderedListMetricSamplerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *OrderedListMetricSamplerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *OrderedListMetricSamplerBuilder) GetOntology() string {
	return "ordered_list_metric_sampler"
}

func init() {
	builders.RegisterBuilder(NewOrderedListMetricSamplerBuilder())
}
