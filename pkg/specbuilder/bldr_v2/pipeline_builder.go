package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// PipelineBuilder builds the pipeline spec at version v2_0_0
// File: bldr_v2/pipeline_builder.go - version is encoded in package/directory name
type PipelineBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPipelineBuilder creates a new builder for pipeline spec version v2_0_0
func NewPipelineBuilder() *PipelineBuilder {
	builder := &PipelineBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("pipeline", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a directed acyclic graph (DAG) of tasks that agents must perform to achieve a higher-level goal.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPipelineFields()

	return builder
}

// addPipelineFields adds the pipeline fields
func (b *PipelineBuilder) addPipelineFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator.").
			AutomationHooks("trace goal alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to system or business goals this pipeline serves.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("valid goal references.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PL-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/security.").
			AutomationHooks("used for pre-flight policy checks.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to policies that govern this pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"compliance",
				"execution checks",
			}).
			Validation("valid policy references.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/architect.").
			AutomationHooks("trace requirement fulfillment by this pipeline.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to requirements driving this pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("valid requirement references.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PL-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stages", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/orchestrator.").
			AutomationHooks("used for routing sequential tasks.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("agent task registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Ordered stages or DAG nodes of execution.").
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
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("PL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trigger", "map").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator.").
			AutomationHooks("used to determine when the pipeline runs.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("cap_orchestrator.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Describes what triggers the pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"execution",
				"scheduling",
			}).
			Validation("structured object defining trigger type and references.").
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
		WithProfileCode("PL-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PipelineBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PipelineBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PipelineBuilder) GetOntology() string {
	return "pipeline"
}

func init() {
	builders.RegisterBuilder(NewPipelineBuilder())
}
