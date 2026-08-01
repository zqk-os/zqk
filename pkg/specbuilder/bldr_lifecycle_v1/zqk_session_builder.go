package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// ZqkSessionLifecycleBuilder builds the zqk_session lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/zqk_session_builder.go - version is encoded in package/directory name
type ZqkSessionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewZqkSessionLifecycleBuilder creates a new builder for zqk_session lifecycle version v1_0_0
func NewZqkSessionLifecycleBuilder() *ZqkSessionLifecycleBuilder {
	builder := &ZqkSessionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("zqk_session", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"active":    0,
				"archived":  100,
				"completed": 100,
				"error":     0,
			},
		})

	// Add statuses and transitions
	builder.addZqkSessionLifecycleData()

	return builder
}

// addZqkSessionLifecycleData adds the zqk_session lifecycle statuses and transitions
func (b *ZqkSessionLifecycleBuilder) addZqkSessionLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "active",
		Display:     "Active",
		Origin:      true,
		Description: "Session is active (CLI run or interactive session in progress).",
	})
	b.AddStatus(objects.Status{
		Value:       "completed",
		Display:     "Completed",
		Terminal:    true,
		Description: "Session completed normally.",
	})
	b.AddStatus(objects.Status{
		Value:       "archived",
		Display:     "Archived",
		Terminal:    true,
		Archive:     true,
		Description: "Session archived (e.g. by retention or manual). Eligible for cleanup.",
	})
	b.AddStatus(objects.Status{
		Value:       "error",
		Display:     "Error",
		System:      true,
		Description: "System error occurred during session.",
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "completed",
		Description: "Session completed normally",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "archived",
		Description: "Archive session (manual or retention)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "completed",
		To:          "archived",
		Description: "Archive completed session",
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
	lifecycle_builders.RegisterBuilder(NewZqkSessionLifecycleBuilder())
}
