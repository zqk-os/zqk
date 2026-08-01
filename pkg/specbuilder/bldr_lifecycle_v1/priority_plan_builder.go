package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// PriorityPlanLifecycleBuilder builds the priority_plan lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/priority_plan_builder.go - version is encoded in package/directory name
type PriorityPlanLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewPriorityPlanLifecycleBuilder creates a new builder for priority_plan lifecycle version v1_0_0
func NewPriorityPlanLifecycleBuilder() *PriorityPlanLifecycleBuilder {
	builder := &PriorityPlanLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("priority_plan", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "prioritizing",
			"archived":    "archived",
			"blocked":     "blocked",
			"error":       "cancelled",
			"grooming":    "grooming",
			"implemented": "complete",
			"in_progress": "active",
			"paused":      "paused",
			"proposed":    "planning",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "item_based",
			DefaultByStatus: map[string]any{
				"active":       "calculated",
				"archived":     100,
				"blocked":      "calculated",
				"cancelled":    0,
				"complete":     100,
				"grooming":     25,
				"paused":       "calculated",
				"planning":     0,
				"prioritizing": 50,
			},
		})

	// Add statuses and transitions
	builder.addPriorityPlanLifecycleData()

	return builder
}

// addPriorityPlanLifecycleData adds the priority_plan lifecycle statuses and transitions
func (b *PriorityPlanLifecycleBuilder) addPriorityPlanLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "planning",
		Display:     "Planning",
		Origin:      true,
		Description: "Initial planning phase - queues planning template for creating new priority plan",
	})
	b.AddStatus(objects.Status{
		Value:       "grooming",
		Display:     "Grooming",
		Description: "Grooming phase - queues grooming template for evaluating backlog alignment",
	})
	b.AddStatus(objects.Status{
		Value:       "prioritizing",
		Display:     "Prioritizing",
		Description: "Prioritization phase - queues prioritization template for assigning priority tiers",
	})
	b.AddStatus(objects.Status{
		Value:       "active",
		Display:     "Active",
		Description: "Priority plan is active and being executed",
	})
	b.AddStatus(objects.Status{
		Value:       "in_progress",
		Display:     "In Progress",
		Description: "Priority plan is in progress (execution locked — no scope expansion/new BLIs allowed)",
	})
	b.AddStatus(objects.Status{
		Value:       "paused",
		Display:     "Paused",
		Description: "Priority plan is temporarily paused",
	})
	b.AddStatus(objects.Status{
		Value:       "blocked",
		Display:     "Blocked",
		Description: "Priority plan is blocked by dependencies",
	})
	b.AddStatus(objects.Status{
		Value:       "complete",
		Display:     "Complete",
		Terminal:    true,
		Description: "Priority plan is complete - all items have been executed or moved to next plan",
	})
	b.AddStatus(objects.Status{
		Value:       "cancelled",
		Display:     "Cancelled",
		Terminal:    true,
		Description: "Priority plan was cancelled before completion",
	})
	b.AddStatus(objects.Status{
		Value:       "archived",
		Display:     "Archived",
		Terminal:    true,
		Archive:     true,
		Description: "Priority plan is archived for historical reference",
	})

	b.AddTransition(objects.Transition{
		From:        "planning",
		To:          "grooming",
		Description: "Move from planning to grooming phase - queues grooming template",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Planning template completed",
			"North star defined",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "grooming",
		To:          "prioritizing",
		Description: "Move from grooming to prioritization phase - queues prioritization template",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Grooming template completed",
			"Backlog alignment analysis done",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "prioritizing",
		To:          "active",
		Description: "Activate priority plan after prioritization is complete (staged as shovel-ready)",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Prioritization template completed",
			"At least one backlog_item_ref linked",
			"Priority plan validated",
			"Workflow constraints validated (if workflow_ref is set)",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "in_progress",
		Description: "Auto-transition from active to in_progress when the first linked backlog_item starts work (locks scope & clears active execution order position)",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "active",
		Description: "Transition from in_progress back to active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "paused",
		Description: "Pause active priority plan",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "paused",
		Description: "Pause in_progress priority plan",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "active",
		Description: "Resume paused priority plan to active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "in_progress",
		Description: "Resume paused priority plan to in_progress",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "blocked",
		Description: "Block active priority plan due to dependencies",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "blocked",
		Description: "Block in_progress priority plan due to dependencies",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "active",
		Description: "Unblock priority plan when dependencies resolved",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "prioritizing",
		To:          "grooming",
		Description: "Dependency propagation — linked work changed while plan is still in prioritization; return to grooming",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "grooming",
		Description: "Dependency propagation — linked backlog work reopened; return plan to grooming for alignment",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "grooming",
		Description: "Dependency propagation — linked backlog work reopened; return plan to grooming for alignment",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "grooming",
		Description: "Dependency propagation — linked backlog work reopened; return plan to grooming for alignment",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "grooming",
		Description: "Dependency propagation — linked backlog work reopened; return plan to grooming for alignment",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Auto-transition when all items complete or moved to next plan",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "complete",
		Description: "Auto-transition when all items complete or moved to next plan",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "cancelled",
		Description: "Cancel active priority plan",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "cancelled",
		Description: "Cancel in_progress priority plan",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "cancelled",
		Description: "Cancel paused priority plan",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "cancelled",
		Description: "Cancel blocked priority plan",
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
		From:        "archived",
		To:          "in_progress",
		Description: "Restore archived priority plan",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewPriorityPlanLifecycleBuilder())
}
