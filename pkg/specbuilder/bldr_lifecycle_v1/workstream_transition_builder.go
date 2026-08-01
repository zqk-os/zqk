package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// WorkstreamTransitionLifecycleBuilder builds the workstream_transition lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/workstream_transition_builder.go - version is encoded in package/directory name
type WorkstreamTransitionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewWorkstreamTransitionLifecycleBuilder creates a new builder for workstream_transition lifecycle version v1_0_0
func NewWorkstreamTransitionLifecycleBuilder() *WorkstreamTransitionLifecycleBuilder {
	builder := &WorkstreamTransitionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("workstream_transition", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"active":   100,
				"archived": 100,
				"complete": 100,
				"error":    0,
				"pending":  0,
			},
		})

	// Add statuses and transitions
	builder.addWorkstreamTransitionLifecycleData()

	return builder
}

// addWorkstreamTransitionLifecycleData adds the workstream_transition lifecycle statuses and transitions
func (b *WorkstreamTransitionLifecycleBuilder) addWorkstreamTransitionLifecycleData() {

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
		From:        "planned",
		To:          "active",
		Description: "Activate transition",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Complete transition",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "complete",
		To:          "archived",
		Description: "Archive transition",
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
	lifecycle_builders.RegisterBuilder(NewWorkstreamTransitionLifecycleBuilder())
}
