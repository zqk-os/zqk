package bldr_v2

import (
	metricbldr "github.com/zqk-os/zqk/packs/metric/bldr_v2"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TestAuditAggregationMetricBuilder builds the test_audit_aggregation_metric spec at version v2_0_0
// File: bldr_v2/test_audit_aggregation_metric_builder.go - version is encoded in package/directory name
type TestAuditAggregationMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTestAuditAggregationMetricBuilder creates a new builder for test_audit_aggregation_metric spec version v2_0_0
func NewTestAuditAggregationMetricBuilder() *TestAuditAggregationMetricBuilder {
	builder := &TestAuditAggregationMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("test_audit_aggregation_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("TEST-ONLY: Aggregated metrics derived from audit events. This is a test-specific spec with different ID prefix to avoid polluting project data. Used for testing content-addressable storage implementation. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addTestAuditAggregationMetricFields()

	return builder
}

// addTestAuditAggregationMetricFields adds the test_audit_aggregation_metric fields
func (b *TestAuditAggregationMetricBuilder) addTestAuditAggregationMetricFields() {
	metricbldr.AddAuditAggregationMetricFields(b.BaseSpecBuilder)
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TestAuditAggregationMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TestAuditAggregationMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TestAuditAggregationMetricBuilder) GetOntology() string {
	return "test_audit_aggregation_metric"
}

func init() {
	builders.RegisterBuilder(NewTestAuditAggregationMetricBuilder())
}
