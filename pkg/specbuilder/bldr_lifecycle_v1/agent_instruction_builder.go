package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// AgentInstructionLifecycleBuilder builds the agent_instruction lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/agent_instruction_builder.go - version is encoded in package/directory name
type AgentInstructionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewAgentInstructionLifecycleBuilder creates a new builder for agent_instruction lifecycle version v1_0_0
func NewAgentInstructionLifecycleBuilder() *AgentInstructionLifecycleBuilder {
	builder := &AgentInstructionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("agent_instruction", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetExtends("base_lifecycle").
		SetStatusMapping(map[string]string{
			"approved":    "approved",
			"archived":    "completed",
			"error":       "error",
			"implemented": "completed",
			"in_progress": "in_progress",
			"proposed":    "proposed",
		}).
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"approved":    10,
				"completed":   100,
				"error":       0,
				"in_progress": 50,
				"proposed":    0,
				"rejected":    100,
			},
		})

	// Add statuses and transitions
	builder.addAgentInstructionLifecycleData()

	return builder
}

// addAgentInstructionLifecycleData adds the agent_instruction lifecycle statuses and transitions
func (b *AgentInstructionLifecycleBuilder) addAgentInstructionLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "proposed",
		Display: "Proposed",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "approved",
		Display: "Approved",
		Preconditions: []string{
			"title must not be \"required at creation\"",
		},
	})
	b.AddStatus(objects.Status{
		Value:    "rejected",
		Display:  "Rejected",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
		Preconditions: []string{
			"title must not be \"required at creation\"",
		},
	})
	b.AddStatus(objects.Status{
		Value:    "completed",
		Display:  "Completed",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "proposed",
		To:          "approved",
		Description: "Human operator approves the instruction",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "proposed",
		To:          "rejected",
		Description: "Human operator rejects the instruction",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "approved",
		To:          "in_progress",
		Description: "Agent begins executing the instruction",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "completed",
		Description: "Agent completes the instruction successfully",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "error",
		Description: "Agent encounters an error during execution",
		Manual:      true,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "error",
		To:          "in_progress",
		Description: "Retry the instruction after fixing the error",
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
	lifecycle_builders.RegisterBuilder(NewAgentInstructionLifecycleBuilder())
}
