package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// DecisionLifecycleBuilder builds the decision lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/decision_builder.go - version is encoded in package/directory name
type DecisionLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewDecisionLifecycleBuilder creates a new builder for decision lifecycle version v1_0_0
func NewDecisionLifecycleBuilder() *DecisionLifecycleBuilder {
	builder := &DecisionLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("decision", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addDecisionLifecycleData()

	return builder
}

// addDecisionLifecycleData adds the decision lifecycle statuses and transitions
func (b *DecisionLifecycleBuilder) addDecisionLifecycleData() {

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
		Value:    "approved",
		Display:  "Approved",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "rejected",
		Display:  "Rejected",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
	})

	b.AddTransition(objects.Transition{
		From:        "draft",
		To:          "under_review",
		Description: "Submit draft for review",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "active",
		Description: "Activate decision (move to active)",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "approved",
		Description: "Approve decision",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "approved",
		Description: "Approve from active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "under_review",
		To:          "rejected",
		Description: "Reject decision",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "active",
		To:          "rejected",
		Description: "Reject from active",
		Manual:      true,
		Auto:        false,
	})

	b.AddTransition(objects.Transition{
		From:        "*",
		To:          "archived",
		Description: "Manual archival from any status",
		Manual:      true,
		Auto:        false,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewDecisionLifecycleBuilder())
}
