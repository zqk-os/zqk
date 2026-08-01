package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// BacklogItemLifecycleBuilder builds the backlog_item lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/backlog_item_builder.go - version is encoded in package/directory name
type BacklogItemLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewBacklogItemLifecycleBuilder creates a new builder for backlog_item lifecycle version v1_0_0
func NewBacklogItemLifecycleBuilder() *BacklogItemLifecycleBuilder {
	builder := &BacklogItemLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("backlog_item", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "validated",
			"archived":    "archived",
			"deferred":    "deferred",
			"error":       "error",
			"implemented": "complete",
			"in_progress": "in_progress",
			"proposed":    "exploring",
			"roadmap":     "roadmap",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "milestone_based",
			DefaultByStatus: map[string]any{
				"archived":    100,
				"complete":    100,
				"deferred":    0,
				"error":       0,
				"exploring":   0,
				"in_progress": 50,
				"planned":     25,
				"rejected":    0,
				"roadmap":     5,
				"validated":   10,
			},
			MilestoneBased: map[string]any{
				"calculation":         "(completed_milestones / total_milestones) * 100",
				"fallback":            "status_defaults",
				objects.FieldKeyField: "milestone_refs",
			},
		})

	// Add statuses and transitions
	builder.addBacklogItemLifecycleData()

	return builder
}

// addBacklogItemLifecycleData adds the backlog_item lifecycle statuses and transitions
func (b *BacklogItemLifecycleBuilder) addBacklogItemLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "exploring",
		Display: "Exploring",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "validated",
		Display: "Validated",
	})
	b.AddStatus(objects.Status{
		Value:   "roadmap",
		Display: "Roadmap",
	})
	b.AddStatus(objects.Status{
		Value:   "deferred",
		Display: "Deferred",
	})
	b.AddStatus(objects.Status{
		Value:   "planned",
		Display: "Planned",
		Preconditions: []string{
			"priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)",
			"At least one milestone_ref linked",
			"priority_tier is set",
		},
		Description: "Committed on a priority plan with at least one milestone linked. Milestones anchor work to the\\nstrategic hierarchy (mission, vision, goals, strategic plan) so execution is traceable before\\nactive development.\\n",
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
		Preconditions: []string{
			"priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)",
			"At least one milestone_ref linked",
			"priority_tier is set",
			"estimated_effort is set",
		},
		Description: "Active work in flight. Requires the same traceability as planned: priority plan plus milestone\\nlinkage so in_progress can be held without lifecycle error and every active item ties back\\nthrough milestones to mission, vision, goals, and strategic plan.\\n",
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
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
		Archive:  true,
	})
	b.AddStatus(objects.Status{
		Value:    "rejected",
		Display:  "Rejected",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "exploring",
		To:          "validated",
		Description: "Idea reviewed and approved for deeper planning",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Problem statement and acceptance considerations defined",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "exploring",
		To:          "roadmap",
		Description: "Move item to roadmap for future planning",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "exploring",
		To:          "deferred",
		Description: "Defer item for later consideration",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "roadmap",
		To:          "validated",
		Description: "Move item from roadmap to validated",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "roadmap",
		To:          "planned",
		Description: "Move item from roadmap to planned",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"priority_plan_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "deferred",
		To:          "exploring",
		Description: "Move deferred item back to exploring",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "deferred",
		To:          "validated",
		Description: "Move deferred item to validated",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "exploring",
		To:          "rejected",
		Description: "Idea closed after review",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "exploring",
		To:          "planned",
		Description: "Fast-track plan assignment when validation happens during promotion",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Owner confirms validation criteria met during promotion",
			"priority_plan_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "roadmap",
		To:          "planned",
		Description: "Move from roadmap to planned",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"priority_plan_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "validated",
		To:          "planned",
		Description: "Item assigned to priority plan",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"Priority assigned (high/medium/low)",
			"Owner identified",
			"priority_plan_ref is set (required per DEC-priority-plan-ref-requirement)",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "error",
		To:          "planned",
		Description: "Recover from validation error status",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "planned",
		To:          "in_progress",
		Description: "Active development begins",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"At least one active milestone_ref linked",
			"priority_plan_ref is set",
			"priority_plan_ref target must be in active status",
			"milestone_refs must link back to goal_refs",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "complete",
		Description: "Auto-complete when validation and acceptance criteria are met",
		Manual:      true,
		Auto:        true,
		Preconditions: []string{
			"commit_refs is not empty",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "planned",
		Description: "Work paused and returned to plan",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"priority_plan_ref is set",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "complete",
		To:          "archived",
		Description: "Manual archival after completion review",
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
		From:        "*",
		To:          "error",
		Description: "System-assigned when lifecycle incoherency detected",
		Manual:      false,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewBacklogItemLifecycleBuilder())
}
