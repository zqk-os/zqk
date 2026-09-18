package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// BaseMetricEnabledGroupBuilder builds the base_metric_enabled_group trait at version v1_0_0
// File: bldr_trait_v1/base_metric_enabled_group_builder.go - version is encoded in package/directory name
type BaseMetricEnabledGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewBaseMetricEnabledGroupBuilder creates a new builder for base_metric_enabled_group trait version v1_0_0
func NewBaseMetricEnabledGroupBuilder() *BaseMetricEnabledGroupBuilder {
	builder := &BaseMetricEnabledGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("base_metric_enabled_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Base trait group for objects and fields that participate in metrics collection.\\nObjects/fields with this trait group can have their data automatically collected\\nand aggregated into metric objects.\\n\\nThis trait group includes:\\n- readable (must be readable to collect metrics)\\n- listable (for querying metric-enabled objects)\\n- filterable (for filtering metric data)\\n- snapable (for snapshot-based metric collection)\\n").
		SetCategory("base-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable").
		AddIncludes("readable").
		AddIncludes("listable").
		AddIncludes("filterable").
		AddIncludes("snapable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewBaseMetricEnabledGroupBuilder())
}
