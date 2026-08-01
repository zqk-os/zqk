package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// StakeholderProfileLifecycleBuilder builds the stakeholder_profile lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/stakeholder_profile_builder.go - version is encoded in package/directory name
type StakeholderProfileLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewStakeholderProfileLifecycleBuilder creates a new builder for stakeholder_profile lifecycle version v1_0_0
func NewStakeholderProfileLifecycleBuilder() *StakeholderProfileLifecycleBuilder {
	builder := &StakeholderProfileLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("stakeholder_profile", "v1_0_0"),
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
	builder.addStakeholderProfileLifecycleData()

	return builder
}

// addStakeholderProfileLifecycleData adds the stakeholder_profile lifecycle statuses and transitions
func (b *StakeholderProfileLifecycleBuilder) addStakeholderProfileLifecycleData() {

	b.AddStatus(objects.Status{
		Value:       "draft",
		Display:     "Draft",
		Origin:      true,
		Description: "Stakeholder profile is being defined",
	})
	b.AddStatus(objects.Status{
		Value:       "active",
		Display:     "Active",
		Description: "Profile is active and used for alignment",
	})
	b.AddStatus(objects.Status{
		Value:       "archived",
		Display:     "Archived",
		Terminal:    true,
		Archive:     true,
		Description: "Profile is no longer active",
	})
	b.AddStatus(objects.Status{
		Value:       "error",
		Display:     "Error",
		System:      true,
		Description: "System error occurred",
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "active",
		Description: "Activate the stakeholder profile",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "archived",
		Description: "Archive the profile",
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
	lifecycle_builders.RegisterBuilder(NewStakeholderProfileLifecycleBuilder())
}
