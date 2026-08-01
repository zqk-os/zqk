package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// RoleLifecycleBuilder builds the role lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/role_builder.go - version is encoded in package/directory name
type RoleLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewRoleLifecycleBuilder creates a new builder for role lifecycle version v1_0_0
func NewRoleLifecycleBuilder() *RoleLifecycleBuilder {
	builder := &RoleLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("role", "v1_0_0"),
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
	builder.addRoleLifecycleData()

	return builder
}

// addRoleLifecycleData adds the role lifecycle statuses and transitions
func (b *RoleLifecycleBuilder) addRoleLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:    "active",
		Display:  "Active",
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
		From:        "draft",
		To:          "active",
		Description: "Activate role",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "archived",
		Description: "Archive role",
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
	lifecycle_builders.RegisterBuilder(NewRoleLifecycleBuilder())
}
