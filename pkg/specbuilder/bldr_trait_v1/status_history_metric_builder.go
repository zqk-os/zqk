package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// StatusHistoryMetricBuilder builds the status_history_metric trait at version v1_0_0
// File: bldr_trait_v1/status_history_metric_builder.go - version is encoded in package/directory name
type StatusHistoryMetricBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewStatusHistoryMetricBuilder creates a new builder for status_history_metric trait version v1_0_0
func NewStatusHistoryMetricBuilder() *StatusHistoryMetricBuilder {
	builder := &StatusHistoryMetricBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("status_history_metric", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait indicating that a field contains status history metric data.\\nThis is a specialized ordered list metric for tracking status transitions over time.\\n\\nStatus history metrics track lifecycle state changes, providing insights into\\nstate transition patterns, durations in each state, and transition frequencies.\\n").
		SetCategory("metric-type").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_metric_enabled_group").
		AddRequires("ordered_list_metric").
		AddRequires("readable").
		AddIncludes("ordered_list_metric")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewStatusHistoryMetricBuilder())
}
