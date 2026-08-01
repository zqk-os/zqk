package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ProbeSpecBuilder builds the probe_spec spec at version v2_0_0
// File: bldr_v2/probe_spec_builder.go - version is encoded in package/directory name
type ProbeSpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewProbeSpecBuilder creates a new builder for probe_spec spec version v2_0_0
func NewProbeSpecBuilder() *ProbeSpecBuilder {
	builder := &ProbeSpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("probe_spec", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Encapsulates reliable, dedicated system queries and log aggregations (e.g., bash pipelines, SQL queries) to provide pre-configured exploratory probes for agents.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addProbeSpecFields()

	return builder
}

// addProbeSpecFields adds the probe_spec fields
func (b *ProbeSpecBuilder) addProbeSpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("command", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("executed by agent tooling or CLI.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The exact command or pipeline to execute.").
			Security("high risk; must be validated to prevent arbitrary execution.").
			SystemUsage([]any{
				"execution",
			}).
			Validation("non-empty string.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PRB-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by LLM to understand probe purpose.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("What this probe accomplishes and when to use it.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prompting",
			}).
			Validation("non-empty.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PRB-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("format", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("parser guidance.").
			Cardinality("one").
			Criticality("composition").
			Default("text").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Format of the output (e.g. json, text, table).").
			Security("non-sensitive").
			SystemUsage([]any{
				"parsing",
			}).
			Validation("string.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PRB-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ProbeSpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ProbeSpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ProbeSpecBuilder) GetOntology() string {
	return "probe_spec"
}

func init() {
	builders.RegisterBuilder(NewProbeSpecBuilder())
}
