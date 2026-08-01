package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// VisionLifecycleBuilder builds the vision lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/vision_builder.go - version is encoded in package/directory name
type VisionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewVisionLifecycleBuilder creates a new builder for vision lifecycle version v1_0_0
func NewVisionLifecycleBuilder() *VisionLifecycleBuilder {
	builder := &VisionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("vision", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"active":   100,
				"archived": 100,
				"draft":    0,
				"error":    0,
			},
		})

	// Add statuses and transitions
	builder.addVisionLifecycleData()

	return builder
}

// addVisionLifecycleData adds the vision lifecycle statuses and transitions
func (b *VisionLifecycleBuilder) addVisionLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
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
		From:        "draft",
		To:          "active",
		Description: "Vision is published and active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "archived",
		Description: "Vision is archived (no longer active)",
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
	lifecycle_builders.RegisterBuilder(NewVisionLifecycleBuilder())
}
