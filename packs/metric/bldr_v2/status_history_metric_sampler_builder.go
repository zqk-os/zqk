package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// StatusHistoryMetricSamplerBuilder builds the status_history_metric_sampler spec at version v2_0_0
// File: bldr_v2/status_history_metric_sampler_builder.go - version is encoded in package/directory name
type StatusHistoryMetricSamplerBuilder struct {
	*builders.BaseSpecBuilder
}

// NewStatusHistoryMetricSamplerBuilder creates a new builder for status_history_metric_sampler spec version v2_0_0
func NewStatusHistoryMetricSamplerBuilder() *StatusHistoryMetricSamplerBuilder {
	builder := &StatusHistoryMetricSamplerBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("status_history_metric_sampler", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_sampler").
		SetDescription("Sampler configuration for status history metric types. Specialized sampler for status/state transition histories. Tracks lifecycle state changes, providing insights into state transition patterns, durations in each state, and transition frequencies. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addStatusHistoryMetricSamplerFields()

	return builder
}

// addStatusHistoryMetricSamplerFields adds the status_history_metric_sampler fields
func (b *StatusHistoryMetricSamplerBuilder) addStatusHistoryMetricSamplerFields() {
	addSamplerBatchAndIntervalFields(b.BaseSpecBuilder)
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"status_history_metric",
			}).
			Required(true).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *StatusHistoryMetricSamplerBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *StatusHistoryMetricSamplerBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *StatusHistoryMetricSamplerBuilder) GetOntology() string {
	return "status_history_metric_sampler"
}

func init() {
	builders.RegisterBuilder(NewStatusHistoryMetricSamplerBuilder())
}
