package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// RequirementLifecycleBuilder builds the requirement lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/requirement_builder.go - version is encoded in package/directory name
type RequirementLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewRequirementLifecycleBuilder creates a new builder for requirement lifecycle version v1_0_0
func NewRequirementLifecycleBuilder() *RequirementLifecycleBuilder {
	builder := &RequirementLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("requirement", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "test_case_based OR milestone_based",
			DefaultByStatus: map[string]any{
				"active":   50,
				"complete": 100,
				"deferred": 0,
				"planned":  0,
				"rejected": 0,
			},
			MilestoneBased: map[string]any{
				"calculation":         "use_milestone_percent_complete",
				objects.FieldKeyField: "milestone_ref",
			},
		})

	// Add statuses and transitions
	builder.addRequirementLifecycleData()

	return builder
}

// addRequirementLifecycleData adds the requirement lifecycle statuses and transitions
func (b *RequirementLifecycleBuilder) addRequirementLifecycleData() {

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
		Value:    "deferred",
		Display:  "Deferred",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "rejected",
		Display:  "Rejected",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "planned",
		To:          "active",
		Description: "Auto-transition when linked milestone transitions to in_progress",
		Manual:      false,
		Auto:        true,
		Preconditions: []string{
			"At least one active milestone_ref linked",
			"At least one active test_case_ref linked",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "complete",
		Description: "Auto-transition when all test cases passing OR linked milestone complete",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "deferred",
		Description: "Manual deferral",
		Manual:      true,
		Auto:        false,
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
	lifecycle_builders.RegisterBuilder(NewRequirementLifecycleBuilder())
}
