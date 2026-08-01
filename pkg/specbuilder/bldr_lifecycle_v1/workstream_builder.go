package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// WorkstreamLifecycleBuilder builds the workstream lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/workstream_builder.go - version is encoded in package/directory name
type WorkstreamLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewWorkstreamLifecycleBuilder creates a new builder for workstream lifecycle version v1_0_0
func NewWorkstreamLifecycleBuilder() *WorkstreamLifecycleBuilder {
	builder := &WorkstreamLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("workstream", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "active",
			"archived":    "archived",
			"error":       "error",
			"implemented": "complete",
			"in_progress": "active",
			"proposed":    "planned",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "milestone_based",
			DefaultByStatus: map[string]any{
				"active":   "calculated",
				"archived": 100,
				"complete": 100,
				"paused":   "calculated",
				"planned":  0,
			},
			MilestoneBased: map[string]any{
				"calculation":            "(completed_milestones / total_milestones) * 100",
				objects.FieldKeyField:    "milestone_refs",
				"weighted_by_stage_type": false,
			},
		})

	// Add statuses and transitions
	builder.addWorkstreamLifecycleData()

	return builder
}

// addWorkstreamLifecycleData adds the workstream lifecycle statuses and transitions
func (b *WorkstreamLifecycleBuilder) addWorkstreamLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "planned",
		Display: "Planned",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
	})
	b.AddStatus(objects.Status{
		Value:   "paused",
		Display: "Paused",
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
		From:        "planned",
		To:          "active",
		Description: "Auto-transition when all prerequisite milestones are complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Auto-transition when all linked milestones are complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "paused",
		Description: "Auto-transition when any linked milestone is blocked for > threshold duration",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "active",
		Description: "Manual resume from paused",
		Manual:      true,
		Auto:        false,
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
	lifecycle_builders.RegisterBuilder(NewWorkstreamLifecycleBuilder())
}
