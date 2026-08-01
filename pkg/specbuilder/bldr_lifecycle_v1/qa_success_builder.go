package bldr_lifecycle_v1

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/lifecycle_builders"
)

// QaSuccessLifecycleBuilder builds the qa_success lifecycle at version v1_0_0
// File: bldr_lifecycle_v1/qa_success_builder.go - version is encoded in package/directory name
type QaSuccessLifecycleBuilder struct {
	*lifecycle_builders.BaseLifecycleBuilder
}

// NewQaSuccessLifecycleBuilder creates a new builder for qa_success lifecycle version v1_0_0
func NewQaSuccessLifecycleBuilder() *QaSuccessLifecycleBuilder {
	builder := &QaSuccessLifecycleBuilder{
		BaseLifecycleBuilder: lifecycle_builders.NewBaseLifecycleBuilder("qa_success", "v1_0_0"),
	}

	// Add statuses and transitions
	builder.addQaSuccessLifecycleData()

	return builder
}

// addQaSuccessLifecycleData adds the qa_success lifecycle statuses and transitions
func (b *QaSuccessLifecycleBuilder) addQaSuccessLifecycleData() {

	b.AddStatus(objects.Status{
		Value:    "success",
		Display:  "Success",
		Origin:   true,
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "failure",
		Display:  "Failure",
		Terminal: true,
	})
	b.AddStatus(objects.Status{
		Value:    "archived",
		Display:  "Archived",
		Terminal: true,
		Archive:  true,
	})
}

func init() {
	lifecycle_builders.RegisterBuilder(NewQaSuccessLifecycleBuilder())
}
