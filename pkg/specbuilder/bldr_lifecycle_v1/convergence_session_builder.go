package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// ConvergenceSessionLifecycleBuilder builds the convergence_session lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/convergence_session_builder.go - version is encoded in package/directory name
type ConvergenceSessionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewConvergenceSessionLifecycleBuilder creates a new builder for convergence_session lifecycle version v1_0_0
func NewConvergenceSessionLifecycleBuilder() *ConvergenceSessionLifecycleBuilder {
	builder := &ConvergenceSessionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("convergence_session", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_based",
			DefaultByStatus: map[string]any{
				"abandoned": 0,
				"active":    40,
				"completed": 100,
				"draft":     0,
				"error":     0,
				"escalated": 50,
				"paused":    35,
			},
		})

	// Add statuses and transitions
	builder.addConvergenceSessionLifecycleData()

	return builder
}

// addConvergenceSessionLifecycleData adds the convergence_session lifecycle statuses and transitions
func (b *ConvergenceSessionLifecycleBuilder) addConvergenceSessionLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "draft",
		Display:     "Draft",
		Origin:      true,
		Description: "Session created; hypothesis and thresholds may be incomplete.",
	})
	b.AddStatus(objects.Status{
		Value:       "active",
		Display:     "Active",
		Description: "Convergence work in progress (phases C1–C5).",
	})
	b.AddStatus(objects.Status{
		Value:       "paused",
		Display:     "Paused",
		Description: "Stopped temporarily; handoff fields should be current.",
	})
	b.AddStatus(objects.Status{
		Value:       "completed",
		Display:     "Completed",
		Terminal:    true,
		Description: "Desired end state reached or accepted stop.",
	})
	b.AddStatus(objects.Status{
		Value:       "abandoned",
		Display:     "Abandoned",
		Terminal:    true,
		Archive:     true,
		Description: "Work discontinued without reaching end state.",
	})
	b.AddStatus(objects.Status{
		Value:       "escalated",
		Display:     "Escalated",
		Terminal:    true,
		Description: "Routed to broader design or ownership outside this session.",
	})
	b.AddStatus(objects.Status{
		Value:       "error",
		Display:     "Error",
		System:      true,
		Description: "System or validation error on this object.",
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "active",
		Description: "Start convergence work",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "abandoned",
		Description: "Discard draft",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "paused",
		Description: "Pause for handoff or dependency",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "completed",
		Description: "Mark converged or done",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "abandoned",
		Description: "Stop without success",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "escalated",
		Description: "Escalate to design or other owner",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "active",
		Description: "Resume",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "paused",
		To:          "abandoned",
		Description: "Abandon while paused",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "System error",
		Manual:      false,
		Auto:        true,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewConvergenceSessionLifecycleBuilder())
}
