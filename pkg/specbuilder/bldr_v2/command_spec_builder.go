package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CommandSpecBuilder builds the command_spec spec at version v2_0_0
// File: bldr_v2/command_spec_builder.go - version is encoded in package/directory name
type CommandSpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCommandSpecBuilder creates a new builder for command_spec spec version v2_0_0
func NewCommandSpecBuilder() *CommandSpecBuilder {
	builder := &CommandSpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("command_spec", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a CLI command in the ZQK ecosystem. Every CLI command (surface area) MUST be generated\\nfrom a command_spec object. These specs enforce uniform CLI handler patterns, documentation,\\nargument validation, and flag definition. By treating commands as system objects, ZQK maintains\\nstrict architectural governance over its own interaction surface.\\nSpec files are traditionally located under `.zqk/cli/specs/`.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("readable")

	// Add fields
	builder.addCommandSpecFields()

	return builder
}

// addCommandSpecFields adds the command_spec fields
func (b *CommandSpecBuilder) addCommandSpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("flags", "list").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("group_id", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("long", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("short", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("use", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CommandSpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CommandSpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CommandSpecBuilder) GetOntology() string {
	return "command_spec"
}

func init() {
	builders.RegisterBuilder(NewCommandSpecBuilder())
}
