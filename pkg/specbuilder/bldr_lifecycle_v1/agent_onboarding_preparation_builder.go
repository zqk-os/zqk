package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// AgentOnboardingPreparationLifecycleBuilder builds the agent_onboarding_preparation lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/agent_onboarding_preparation_builder.go - version is encoded in package/directory name
type AgentOnboardingPreparationLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewAgentOnboardingPreparationLifecycleBuilder creates a new builder for agent_onboarding_preparation lifecycle version v1_0_0
func NewAgentOnboardingPreparationLifecycleBuilder() *AgentOnboardingPreparationLifecycleBuilder {
	builder := &AgentOnboardingPreparationLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("agent_onboarding_preparation", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"active":      100,
				"archived":    100,
				"complete":    100,
				"error":       0,
				"in_progress": 50,
				"pending":     0,
			},
		})

	// Add statuses and transitions
	builder.addAgentOnboardingPreparationLifecycleData()

	return builder
}

// addAgentOnboardingPreparationLifecycleData adds the agent_onboarding_preparation lifecycle statuses and transitions
func (b *AgentOnboardingPreparationLifecycleBuilder) addAgentOnboardingPreparationLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "pending",
		Display: "Pending",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
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
		Archive:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "error",
		Display: "Error",
		System:  true,
	})

	b.AddTransition(objects.Transition{
		From:        "pending",
		To:          "in_progress",
		Description: "Start preparation work",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "active",
		Description: "Mark preparation as active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Complete preparation",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "complete",
		To:          "archived",
		Description: "Archive preparation",
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
	lifecycle_builders.RegisterBuilder(NewAgentOnboardingPreparationLifecycleBuilder())
}
