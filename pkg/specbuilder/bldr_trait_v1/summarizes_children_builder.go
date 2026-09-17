package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// SummarizesChildrenBuilder builds the summarizes_children trait at version v1_0_0.
type SummarizesChildrenBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewSummarizesChildrenBuilder creates a builder for summarizes_children trait version v1_0_0.
func NewSummarizesChildrenBuilder() *SummarizesChildrenBuilder {
	builder := &SummarizesChildrenBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("summarizes_children", "v1_0_0"),
	}

	builder.
		SetDescription("Object-level child-rollup summary capability. Indicates that the container object aggregates and summarizes metrics from its child objects (such as backlog items under a milestone or priority plan). Provides rollup counts, total estimated and actual effort, and distributions (average, min, max).\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false).
		SetConfig(map[string]any{
			"child_kinds":   []string{"backlog_item"},
			"rollup_fields": []string{"estimated_effort", "actual_effort"},
		})

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewSummarizesChildrenBuilder())
}
