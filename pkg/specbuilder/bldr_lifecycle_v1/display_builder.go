package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// DisplayLifecycleBuilder builds the display lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/display_builder.go - version is encoded in package/directory name
type DisplayLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewDisplayLifecycleBuilder creates a new builder for display lifecycle version v1_0_0
func NewDisplayLifecycleBuilder() *DisplayLifecycleBuilder {
	builder := &DisplayLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("display", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"archived":   100,
				"configured": 50,
				"created":    0,
				"error":      0,
				"rendered":   100,
				"validated":  25,
			},
		})

	// Add statuses and transitions
	builder.addDisplayLifecycleData()

	return builder
}

// addDisplayLifecycleData adds the display lifecycle statuses and transitions
func (b *DisplayLifecycleBuilder) addDisplayLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "created",
		Display:     "Created",
		Origin:      true,
		Description: "Display has been instantiated but not yet validated",
	})
	b.AddStatus(objects.Status{
		Value:   "validated",
		Display: "Validated",
		Preconditions: []string{
			"Display type is valid",
			"Configuration passes validation",
			"Constraint spec exists (if constrainable)",
		},
		Description: "Display passes validation checks (type, configuration, constraints)",
	})
	b.AddStatus(objects.Status{
		Value:   "configured",
		Display: "Configured",
		Preconditions: []string{
			"Display has valid layout configuration",
			"Component references are valid",
			"Layout constraints satisfied",
		},
		Description: "Display layout and components are configured",
	})
	b.AddStatus(objects.Status{
		Value:   "rendered",
		Display: "Rendered",
		Preconditions: []string{
			"Display successfully written to output",
			"Output structure is valid",
		},
		Description: "Display successfully written to output",
	})
	b.AddStatus(objects.Status{
		Value:       "archived",
		Display:     "Archived",
		Terminal:    true,
		Archive:     true,
		Description: "Display is no longer active and has been archived",
	})
	b.AddStatus(objects.Status{
		Value:       "error",
		Display:     "Error",
		System:      true,
		Description: "Display encountered an error during validation, configuration, or rendering",
	})

	b.AddTransition(objects.Transition{
		From:        "created",
		To:          "validated",
		Description: "Display passes validation checks",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "validated",
		To:          "configured",
		Description: "Display layout and components are configured",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "configured",
		To:          "rendered",
		Description: "Display successfully rendered to output",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "error",
		Description: "Display encountered an error",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Display is no longer needed and can be archived",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewDisplayLifecycleBuilder())
}
