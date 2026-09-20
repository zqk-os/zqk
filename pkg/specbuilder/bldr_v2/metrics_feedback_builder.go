package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// MetricsFeedbackBuilder builds the metrics_feedback spec at version v2_0_0
// File: bldr_v2/metrics_feedback_builder.go - version is encoded in package/directory name
type MetricsFeedbackBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMetricsFeedbackBuilder creates a new builder for metrics_feedback spec version v2_0_0
func NewMetricsFeedbackBuilder() *MetricsFeedbackBuilder {
	builder := &MetricsFeedbackBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("metrics_feedback", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Captures analysis, historical context, and autonomous improvement suggestions derived from system metrics and reports. Forms the foundation of the report-driven autonomous feedback loop.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addMetricsFeedbackFields()

	return builder
}

// addMetricsFeedbackFields adds the metrics_feedback fields
func (b *MetricsFeedbackBuilder) addMetricsFeedbackFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("analysis", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_refs", "list"))
	b.AddFieldBuilder(builders.NewFieldBuilder("suggested_actions", "list"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_report", "string"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MetricsFeedbackBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MetricsFeedbackBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MetricsFeedbackBuilder) GetOntology() string {
	return "metrics_feedback"
}

func init() {
	builders.RegisterBuilder(NewMetricsFeedbackBuilder())
}
