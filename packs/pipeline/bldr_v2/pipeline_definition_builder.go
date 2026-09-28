package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// PipelineDefinitionBuilder builds the pipeline_definition spec at version v2_0_0
// File: bldr_v2/pipeline_definition_builder.go - version is encoded in package/directory name
type PipelineDefinitionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPipelineDefinitionBuilder creates a new builder for pipeline_definition spec version v2_0_0
func NewPipelineDefinitionBuilder() *PipelineDefinitionBuilder {
	builder := &PipelineDefinitionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("pipeline_definition", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines the template and structure of a pipeline.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPipelineDefinitionFields()

	return builder
}

// addPipelineDefinitionFields adds the pipeline_definition fields
func (b *PipelineDefinitionBuilder) addPipelineDefinitionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("stages", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("execution").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Ordered stages of execution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"orchestration",
			}).
			Validation("list of stage objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("PLD-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("triggers", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("trigger").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Event hooks that initiate the pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduling",
			}).
			Validation("list of trigger objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("PLD-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PipelineDefinitionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PipelineDefinitionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PipelineDefinitionBuilder) GetOntology() string {
	return "pipeline_definition"
}

func init() {
	builders.RegisterBuilder(NewPipelineDefinitionBuilder())
}
