package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// ScalarMetricBuilder builds the scalar_metric trait at version v1_0_0
// File: bldr_trait_v1/scalar_metric_builder.go - version is encoded in package/directory name
type ScalarMetricBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewScalarMetricBuilder creates a new builder for scalar_metric trait version v1_0_0
func NewScalarMetricBuilder() *ScalarMetricBuilder {
	builder := &ScalarMetricBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("scalar_metric", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait indicating that a field contains scalar metric data (single numeric value).\\nFields with this trait are automatically collected and aggregated into metric objects.\\n\\nScalar metrics represent single numeric measurements (e.g., count, duration, size).\\nThey can be aggregated using sum, average, min, max, count operations.\\n").
		SetCategory("metric-type").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_metric_enabled_group").
		AddRequires("readable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewScalarMetricBuilder())
}
