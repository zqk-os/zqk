package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// CriteriaLifecycleBuilder builds the criteria lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/criteria_builder.go - version is encoded in package/directory name
type CriteriaLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewCriteriaLifecycleBuilder creates a new builder for criteria lifecycle version v1_0_0
func NewCriteriaLifecycleBuilder() *CriteriaLifecycleBuilder {
	builder := &CriteriaLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("criteria", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_based",
			DefaultByStatus: map[string]any{
				"blocked":     0,
				"complete":    100,
				"in_progress": 50,
				"not_started": 0,
				"rejected":    0,
				"validated":   90,
			},
		})

	// Add statuses and transitions
	builder.addCriteriaLifecycleData()

	return builder
}

// addCriteriaLifecycleData adds the criteria lifecycle statuses and transitions
func (b *CriteriaLifecycleBuilder) addCriteriaLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "not_started",
		Display: "Not Started",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "in_progress",
		Display: "In Progress",
	})
	b.AddStatus(objects.Status{
		Value:   "validated",
		Display: "Validated",
	})
	b.AddStatus(objects.Status{
		Value:    "complete",
		Display:  "Complete",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:   "blocked",
		Display: "Blocked",
	})
	b.AddStatus(objects.Status{
		Value:    "rejected",
		Display:  "Rejected",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "not_started",
		To:          "in_progress",
		Description: "Manual transition when work on criterion begins",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "validated",
		Description: "Manual transition after validation check",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "complete",
		Description: "Auto-transition when automated test passes",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "complete",
		Description: "Auto-transition when metric threshold is met",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "validated",
		To:          "complete",
		Description: "Manual confirmation after validation",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "in_progress",
		To:          "blocked",
		Description: "Auto-transition when dependent criteria are blocked",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "blocked",
		To:          "in_progress",
		Description: "Auto-transition when all dependent criteria are complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "rejected",
		Description: "Manual rejection from any status",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewCriteriaLifecycleBuilder())
}
