package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// ListMetricBuilder builds the list_metric trait at version v1_0_0
// File: bldr_trait_v1/list_metric_builder.go - version is encoded in package/directory name
type ListMetricBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewListMetricBuilder creates a new builder for list_metric trait version v1_0_0
func NewListMetricBuilder() *ListMetricBuilder {
	builder := &ListMetricBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("list_metric", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait indicating that a field contains list-based metric data (unordered collection).\\nFields with this trait are automatically collected and aggregated into metric objects.\\n\\nList metrics represent collections of values that can be counted, analyzed for\\nfrequency, and aggregated by value distribution.\\n").
		SetCategory("metric-type").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_metric_enabled_group").
		AddRequires("readable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewListMetricBuilder())
}
