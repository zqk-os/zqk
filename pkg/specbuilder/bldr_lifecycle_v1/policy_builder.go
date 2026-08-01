package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// PolicyLifecycleBuilder builds the policy lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/policy_builder.go - version is encoded in package/directory name
type PolicyLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewPolicyLifecycleBuilder creates a new builder for policy lifecycle version v1_0_0
func NewPolicyLifecycleBuilder() *PolicyLifecycleBuilder {
	builder := &PolicyLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("policy", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addPolicyLifecycleData()

	return builder
}

// addPolicyLifecycleData adds the policy lifecycle statuses and transitions
func (b *PolicyLifecycleBuilder) addPolicyLifecycleData() {

	b.AddStatus(objects.Status{
		Value:   "draft",
		Display: "Draft",
		Origin:  true,
	})
	b.AddStatus(objects.Status{
		Value:   "under_review",
		Display: "Under Review",
	})
	b.AddStatus(objects.Status{
		Value:   "active",
		Display: "Active",
	})
	b.AddStatus(objects.Status{
		Value:   "deprecated",
		Display: "Deprecated",
	})
	b.AddStatus(objects.Status{
		Value:   "superseded",
		Display: "Superseded",
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "under_review",
		Description: "Submit policy for review",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"category field populated",
			"policy_type field populated",
			"body field populated",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "active",
		Description: "Activate policy (approved and effective)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "under_review",
		Description: "Policy updated - return to review",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "deprecated",
		Description: "Deprecate policy (no longer applicable but kept for reference)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "superseded",
		Description: "Policy superseded by new policy",
		Manual:      true,
		Auto:        false,
		Preconditions: []string{
			"related_patterns contains reference to superseding policy",
		},
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "active",
		Description: "Policy effective date reached - activate if in review",
		Manual:      false,
		Auto:        true,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Manual archival",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewPolicyLifecycleBuilder())
}
