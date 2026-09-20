package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// PipelineExecutionBuilder builds the pipeline_execution spec at version v2_0_0
// File: bldr_v2/pipeline_execution_builder.go - version is encoded in package/directory name
type PipelineExecutionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPipelineExecutionBuilder creates a new builder for pipeline_execution spec version v2_0_0
func NewPipelineExecutionBuilder() *PipelineExecutionBuilder {
	builder := &PipelineExecutionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("pipeline_execution", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Tracks an active, running instance of a pipeline_definition.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPipelineExecutionFields()

	return builder
}

// addPipelineExecutionFields adds the pipeline_execution fields
func (b *PipelineExecutionBuilder) addPipelineExecutionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("audit_trail", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("audit").
			Cardinality("many").
			Criticality("audit").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("append-only.").
			Observability("yes").
			Purpose("Array of operations performed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit",
			}).
			Validation("list.").
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
		WithProfileCode("PLX-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context_payload", "map").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("payload").
			Cardinality("one").
			Criticality("state").
			Default(map[string]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Arbitrary JSON holding working context.").
			Security("non-sensitive").
			SystemUsage([]any{
				"state",
			}).
			Validation("map.").
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
		WithProfileCode("PLX-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("current_stage", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("execution").
			Cardinality("one").
			Criticality("state").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("String identifier of the active stage.").
			Security("non-sensitive").
			SystemUsage([]any{
				"state",
			}).
			Validation("string.").
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
		WithProfileCode("PLX-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("definition_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator").
			AutomationHooks("linking").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("pipeline_definition registry.").
			Lifecycle("read-only.").
			Observability("yes").
			Purpose("Pointer to the pipeline_definition.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("valid pipeline_definition reference.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_reference_group", "field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PLX-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PipelineExecutionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PipelineExecutionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PipelineExecutionBuilder) GetOntology() string {
	return "pipeline_execution"
}

func init() {
	builders.RegisterBuilder(NewPipelineExecutionBuilder())
}
