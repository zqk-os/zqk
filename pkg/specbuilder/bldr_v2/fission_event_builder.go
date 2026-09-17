package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// FissionEventBuilder builds the fission_event spec at version v2_0_0
// File: bldr_v2/fission_event_builder.go - version is encoded in package/directory name
type FissionEventBuilder struct {
	*builders.BaseSpecBuilder
}

// NewFissionEventBuilder creates a new builder for fission_event spec version v2_0_0
func NewFissionEventBuilder() *FissionEventBuilder {
	builder := &FissionEventBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("fission_event", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Tracks the autonomous division of a kernel node into a parent-child pair.\\nTriggered by cognitive saturation signals.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addFissionEventFields()

	return builder
}

// addFissionEventFields adds the fission_event fields
func (b *FissionEventBuilder) addFissionEventFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("child_node_id", "string").
		WithTraits("readable", "filterable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("parent_node_id", "string").
		WithTraits("readable", "filterable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("reason", "string").
		WithTraits("readable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("snapshot_id", "string").
		WithTraits("readable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithDefault("initiated").
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *FissionEventBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *FissionEventBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *FissionEventBuilder) GetOntology() string {
	return "fission_event"
}

func init() {
	builders.RegisterBuilder(NewFissionEventBuilder())
}
