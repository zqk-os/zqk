package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// MilestoneLifecycleBuilder builds the milestone lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/milestone_builder.go - version is encoded in package/directory name
type MilestoneLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewMilestoneLifecycleBuilder creates a new builder for milestone lifecycle version v1_0_0
func NewMilestoneLifecycleBuilder() *MilestoneLifecycleBuilder {
	builder := &MilestoneLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("milestone", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "completion_criteria_based",
			DefaultByStatus: map[string]any{
				"archived":    100,
				"blocked":     0,
				"complete":    100,
				"deferred":    0,
				"in_progress": 50,
				"not_started": 0,
			},
		})

	// Add statuses and transitions
	builder.addMilestoneLifecycleData()

	return builder
}

// addMilestoneLifecycleData adds the milestone lifecycle statuses and transitions
func (b *MilestoneLifecycleBuilder) addMilestoneLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "not_started",
		Display: "Not Started",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
		Preconditions: []string{
			"estimated_effort is set",
		},
	})
	b.AddStatus(objects.Status{
		Value:   "blocked",
		Display: "Blocked",
	})
	b.AddStatus(objects.Status{
		Value:    "complete",
		Display:  "Complete",
		Terminal: true,
		Preconditions: []string{
			"estimated_effort is set",
			"actual_effort is set",
		},
	})
	b.AddStatus(objects.Status{
		Value:    "deferred",
		Display:  "Deferred",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "not_started",
		To:          "in_progress",
		Description: "Auto-transition when any linked requirement becomes active",
		Manual:      false,
		Auto:        true,
		Preconditions: []string{
			"At least one active criteria_ref linked",
			"At least one active goal_ref linked",
			"criteria_refs must link back to goal_refs",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "complete",
		Description: "Auto-transition when all completion criteria are checked",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "blocked",
		Description: "Auto-transition when any prerequisite milestone is blocked or deferred",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "in_progress",
		Description: "Auto-transition when all prerequisites are complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "deferred",
		Description: "Manual deferral from any status",
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

	b.AddTransition(objects.Transition{
		From:        "complete",
		To:          "in_progress",
		Description: "Manual reopening when new work is linked to a recently closed milestone",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewMilestoneLifecycleBuilder())
}
