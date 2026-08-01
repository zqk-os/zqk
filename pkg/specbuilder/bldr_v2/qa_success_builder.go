package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// QaSuccessBuilder builds the qa_success spec at version v2_0_0
// File: bldr_v2/qa_success_builder.go - version is encoded in package/directory name
type QaSuccessBuilder struct {
	*builders.BaseSpecBuilder
}

// NewQaSuccessBuilder creates a new builder for qa_success spec version v2_0_0
func NewQaSuccessBuilder() *QaSuccessBuilder {
	builder := &QaSuccessBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("qa_success", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("null").
		SetDescription("Cryptographically signed proof of successful QA audit.").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	return builder
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *QaSuccessBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *QaSuccessBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *QaSuccessBuilder) GetOntology() string {
	return "qa_success"
}

func init() {
	builders.RegisterBuilder(NewQaSuccessBuilder())
}
