package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// GoalLifecycleBuilder builds the goal lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/goal_builder.go - version is encoded in package/directory name
type GoalLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewGoalLifecycleBuilder creates a new builder for goal lifecycle version v1_0_0
func NewGoalLifecycleBuilder() *GoalLifecycleBuilder {
	builder := &GoalLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("goal", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "active",
			"archived":    "archived",
			"blocked":     "blocked",
			"error":       "error",
			"implemented": "complete",
			"in_progress": "active",
			"proposed":    "planned",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "metric_based OR workstream_based",
			DefaultByStatus: map[string]any{
				"active":   "calculated",
				"archived": 100,
				"blocked":  "calculated",
				"complete": 100,
				"planned":  0,
			},
		})

	// Add statuses and transitions
	builder.addGoalLifecycleData()

	return builder
}

// addGoalLifecycleData adds the goal lifecycle statuses and transitions
func (b *GoalLifecycleBuilder) addGoalLifecycleData() {

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
		Value:   "blocked",
		Display: "Blocked",
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
		Description: "Auto-transition when any linked workstream becomes active",
		Manual:      false,
		Auto:        true,
		Preconditions: []string{
			"At least one milestone_ref linked",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Auto-transition when metric target met OR all workstreams complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "blocked",
		Description: "Auto-transition when blockers detected",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "active",
		Description: "Auto-transition when blockers resolved",
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

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "planned",
		Description: "Manual recovery to planned state",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewGoalLifecycleBuilder())
}
