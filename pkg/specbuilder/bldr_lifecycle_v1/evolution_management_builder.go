package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// EvolutionManagementLifecycleBuilder builds the evolution_management lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/evolution_management_builder.go - version is encoded in package/directory name
type EvolutionManagementLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewEvolutionManagementLifecycleBuilder creates a new builder for evolution_management lifecycle version v1_0_0
func NewEvolutionManagementLifecycleBuilder() *EvolutionManagementLifecycleBuilder {
	builder := &EvolutionManagementLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("evolution_management", "v1_0_0"),
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
				"planning": 0,
			},
		})

	// Add statuses and transitions
	builder.addEvolutionManagementLifecycleData()

	return builder
}

// addEvolutionManagementLifecycleData adds the evolution_management lifecycle statuses and transitions
func (b *EvolutionManagementLifecycleBuilder) addEvolutionManagementLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "planning",
		Display: "Planning",
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
		From:        "planning",
		To:          "active",
		Description: "Activate evolution management",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Complete evolution",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "complete",
		To:          "archived",
		Description: "Archive evolution",
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
	lifecycle_builders.RegisterBuilder(NewEvolutionManagementLifecycleBuilder())
}
