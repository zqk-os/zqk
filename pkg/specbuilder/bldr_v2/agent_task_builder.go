package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AgentTaskBuilder builds the agent_task spec at version v2_0_0
// File: bldr_v2/agent_task_builder.go - version is encoded in package/directory name
type AgentTaskBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentTaskBuilder creates a new builder for agent_task spec version v2_0_0
func NewAgentTaskBuilder() *AgentTaskBuilder {
	builder := &AgentTaskBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_task", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_unit").
		AddCompose("occupancy").
		SetDescription("Lifecycle: agent_task_lifecycle.yaml.\\nA discrete unit of work assigned to a specific persona. It serves as the primary routing envelope.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("effort_aware").
		AddTrait("occupiable")

	// Add fields
	builder.addAgentTaskFields()

	return builder
}

// addAgentTaskFields adds the agent_task fields
func (b *AgentTaskBuilder) addAgentTaskFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("assignee_persona_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/tpm.").
			AutomationHooks("used for routing work to specific agents.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("persona registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the persona assigned to execute this task.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"queueing",
			}).
			Validation("valid persona reference.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("AT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("inputs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator.").
			AutomationHooks("populates agent context.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to objects or specs the agent requires to complete work.").
			Security("context-dependent.").
			SystemUsage([]any{
				"context injection",
			}).
			Validation("list of input definitions.").
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
		WithProfileCode("AT-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("model_tier", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator.").
			AutomationHooks("Used by the CAP router to assign task difficulty levels.").
			Cardinality("one").
			Criticality("metadata").
			Default("tier_2_simple").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The model tier required to execute this task (e.g. tier_1_complex, tier_2_simple).").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
			}).
			Validation("Must be a string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AT-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("outputs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("agent.").
			AutomationHooks("registered upon completion for downstream tasks.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to objects or artifacts the agent produces.").
			Security("context-dependent.").
			SystemUsage([]any{
				"downstream routing",
			}).
			Validation("list of output definitions.").
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
		WithProfileCode("AT-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("pipeline_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator.").
			AutomationHooks("connects tasks back to the parent workflow.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("pipeline registry.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the parent pipeline.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"orchestration",
			}).
			Validation("valid pipeline reference.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AT-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/security.").
			AutomationHooks("used for task-level policy checks.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to policies that govern this task.").
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
		WithProfileCode("AT-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/architect.").
			AutomationHooks("trace requirement fulfillment by this task.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to requirements driving this task.").
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
		WithProfileCode("AT-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("validation_criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("architect/qa.").
			AutomationHooks("triggered on task completion proposal.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to criteria objects that must pass for completion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"testing",
			}).
			Validation("valid criteria references.").
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
		WithProfileCode("AT-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority_plan_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/tpm.").
			AutomationHooks("connects tasks back to the parent priority plan and integration branch.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("priority_plan registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the priority plan this task executes within.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"orchestration",
				"git branch resolution",
			}).
			Validation("valid priority_plan reference.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(PRI-[\w-]+|PRIO-[\w-]+|priority-plan-[\w-]+)$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("AT-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("task_steps", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("orchestrator/planner.").
			AutomationHooks("used for step-by-step task execution and verification strategies.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Ordered list of sub-steps and verification criteria required to complete the task.").
			Security("non-sensitive").
			SystemUsage([]any{
				"execution",
				"verification",
				"progress tracking",
			}).
			Validation("list of step definitions.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("AT-010"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentTaskBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentTaskBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentTaskBuilder) GetOntology() string {
	return "agent_task"
}

func init() {
	builders.RegisterBuilder(NewAgentTaskBuilder())
}
