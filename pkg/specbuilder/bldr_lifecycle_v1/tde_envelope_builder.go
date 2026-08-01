package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// TdeEnvelopeLifecycleBuilder builds the tde_envelope lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/tde_envelope_builder.go - version is encoded in package/directory name
type TdeEnvelopeLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewTdeEnvelopeLifecycleBuilder creates a new builder for tde_envelope lifecycle version v1_0_0
func NewTdeEnvelopeLifecycleBuilder() *TdeEnvelopeLifecycleBuilder {
	builder := &TdeEnvelopeLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("tde_envelope", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_defaults",
			DefaultByStatus: map[string]any{
				"approved": 100,
				"error":    0,
				"pending":  0,
				"rejected": 100,
			},
		})

	// Add statuses and transitions
	builder.addTdeEnvelopeLifecycleData()

	return builder
}

// addTdeEnvelopeLifecycleData adds the tde_envelope lifecycle statuses and transitions
func (b *TdeEnvelopeLifecycleBuilder) addTdeEnvelopeLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "pending",
		Display: "Pending",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "approved",
		Display: "Approved",
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
		From:        "pending",
		To:          "approved",
		Description: "Approved for execution",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "pending",
		To:          "rejected",
		Description: "Rejected",
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
	lifecycle_builders.RegisterBuilder(NewTdeEnvelopeLifecycleBuilder())
}
