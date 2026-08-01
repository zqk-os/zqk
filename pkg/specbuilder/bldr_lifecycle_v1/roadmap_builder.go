package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// RoadmapLifecycleBuilder builds the roadmap lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/roadmap_builder.go - version is encoded in package/directory name
type RoadmapLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewRoadmapLifecycleBuilder creates a new builder for roadmap lifecycle version v1_0_0
func NewRoadmapLifecycleBuilder() *RoadmapLifecycleBuilder {
	builder := &RoadmapLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("roadmap", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "milestone_based",
			DefaultByStatus: map[string]any{
				"active":    "calculated",
				"archived":  100,
				"complete":  100,
				"draft":     0,
				"published": "calculated",
			},
			MilestoneBased: map[string]any{
				"calculation":            "(completed_milestones / total_milestones) * 100",
				objects.FieldKeyField:    "milestone_refs",
				"weighted_by_stage_type": false,
			},
		})

	// Add statuses and transitions
	builder.addRoadmapLifecycleData()

	return builder
}

// addRoadmapLifecycleData adds the roadmap lifecycle statuses and transitions
func (b *RoadmapLifecycleBuilder) addRoadmapLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
		Preconditions: []string{
			"At least one workstream_ref or milestone_ref linked (required for roadmap structure)",
		},
	})
	b.AddStatus(objects.Status{
		Value:   "published",
		Display: "Published",
		Preconditions: []string{
			"At least one workstream_ref or milestone_ref linked (required for roadmap structure)",
		},
	})
	b.AddStatus(objects.Status{
		Value:    "complete",
		Display:  "Complete",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "published",
		Description: "Manual publication from draft",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"At least one workstream_ref or milestone_ref linked (required for roadmap structure)",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "published",
		To:          "active",
		Description: "Manual activation from published",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Auto-transition when all linked milestones are complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Manual archival from any status",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewRoadmapLifecycleBuilder())
}
