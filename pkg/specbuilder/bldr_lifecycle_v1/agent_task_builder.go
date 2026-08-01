package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// AgentTaskLifecycleBuilder builds the agent_task lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/agent_task_builder.go - version is encoded in package/directory name
type AgentTaskLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewAgentTaskLifecycleBuilder creates a new builder for agent_task lifecycle version v1_0_0
func NewAgentTaskLifecycleBuilder() *AgentTaskLifecycleBuilder {
	builder := &AgentTaskLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("agent_task", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "in_progress",
			"archived":    "archived",
			"error":       "error",
			"implemented": "completed",
			"in_progress": "in_progress",
			"proposed":    "proposed",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"approved":             10,
				"archived":             100,
				"completed":            100,
				"error":                0,
				"implemented":          100,
				"in_progress":          50,
				"pending_verification": 90,
				"proposed":             0,
			},
		})

	// Add statuses and transitions
	builder.addAgentTaskLifecycleData()

	return builder
}

// addAgentTaskLifecycleData adds the agent_task lifecycle statuses and transitions
func (b *AgentTaskLifecycleBuilder) addAgentTaskLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "proposed",
		Display: "Proposed",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "approved",
		Display: "Approved",
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
		Preconditions: []string{
			"estimated_effort is set",
		},
	})
	b.AddStatus(objects.Status{
		Value:   "pending_verification",
		Display: "Pending Verification",
	})
	b.AddStatus(objects.Status{
		Value:    "implemented",
		Display:  "Implemented",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "completed",
		Display:  "Completed",
		Terminal: true,
		Preconditions: []string{
			"estimated_effort is set",
			"actual_effort is set",
		},
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
		Archive:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "proposed",
		To:          "approved",
		Description: "Legacy approval",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "approved",
		To:          "in_progress",
		Description: "Begin work on task",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "proposed",
		To:          "in_progress",
		Description: "Begin work on task",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "pending_verification",
		Description: "Task is complete, awaiting verification",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "implemented",
		Description: "Legacy completion",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "pending_verification",
		To:          "completed",
		Description: "Task verified and complete",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "pending_verification",
		To:          "implemented",
		Description: "Task verified and complete (legacy/alternative)",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "pending_verification",
		To:          "error",
		Description: "Task verification failed",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "error",
		To:          "in_progress",
		Description: "Wake agent to fix task",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "implemented",
		To:          "in_progress",
		Description: "Reset implemented task back to work",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "implemented",
		To:          "archived",
		Description: "Archive legacy task",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "completed",
		To:          "archived",
		Description: "Archive completed task",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System error occurred",
		Manual:      false,
		Auto:        true,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewAgentTaskLifecycleBuilder())
}
