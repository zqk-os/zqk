package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// AccountLifecycleBuilder builds the account lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/account_builder.go - version is encoded in package/directory name
type AccountLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewAccountLifecycleBuilder creates a new builder for account lifecycle version v1_0_0
func NewAccountLifecycleBuilder() *AccountLifecycleBuilder {
	builder := &AccountLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("account", "v1_0_0"),
	}

	// Configure the lifecycle
	builder.
		SetPercentComplete(objects.PercentCompleteConfig{
			Method: "status_based",
			DefaultByStatus: map[string]any{
				"active":    100,
				"inactive":  0,
				"suspended": 0,
			},
		})

	// Add statuses and transitions
	builder.addAccountLifecycleData()

	return builder
}

// addAccountLifecycleData adds the account lifecycle statuses and transitions
func (b *AccountLifecycleBuilder) addAccountLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "inactive",
		Display: "Inactive",
	})
	b.AddStatus(objects.Status{
		Value:   "suspended",
		Display: "Suspended",
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "inactive",
		Description: "",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "suspended",
		Description: "",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "inactive",
		To:          "active",
		Description: "",
		Manual:      false,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "suspended",
		To:          "active",
		Description: "",
		Manual:      false,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewAccountLifecycleBuilder())
}
