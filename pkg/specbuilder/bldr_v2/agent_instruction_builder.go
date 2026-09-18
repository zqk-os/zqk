package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AgentInstructionBuilder builds the agent_instruction spec at version v2_0_0
// File: bldr_v2/agent_instruction_builder.go - version is encoded in package/directory name
type AgentInstructionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentInstructionBuilder creates a new builder for agent_instruction spec version v2_0_0
func NewAgentInstructionBuilder() *AgentInstructionBuilder {
	builder := &AgentInstructionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_instruction", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("agent_instruction objects represent proposals or executable instructions submitted to the Autonomy Inbox.\\nHuman operators review, approve, or reject these proposals before execution.\\nLifecycle: agent_instruction_lifecycle.yaml\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("constrainable")

	// Add fields
	builder.addAgentInstructionFields()

	return builder
}

// addAgentInstructionFields adds the agent_instruction fields
func (b *AgentInstructionBuilder) addAgentInstructionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("execution_log", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("updated during execution.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Log of execution steps and results.").
			Security("non-sensitive").
			SystemUsage([]any{
				"debugging",
				"auditing",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGI-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("instruction", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used by agents for execution context.").
			Cardinality("one").
			Criticality("composition").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The actual prompt or proposal instruction text for the agent to execute.").
			Security("non-sensitive").
			SystemUsage([]any{
				"execution",
				"context",
			}).
			Validation("free text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGI-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_session", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for traceability back to a convergence session.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Dependencies("convergence session registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Convergence session that generated this instruction.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("string ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AGI-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("lifecycle rules.").
			AutomationHooks("lifecycle transitions.").
			Cardinality("one").
			Criticality("composition").
			Default("proposed").
			Dependencies("lifecycle registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Lifecycle status.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle enforcement",
				"filtering",
			}).
			Validation("Must match lifecycle definition.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"proposed",
				"approved",
				"rejected",
				"in_progress",
				"completed",
				"error",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGI-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_persona", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for routing instructions to the right agent persona.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Dependencies("persona registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional persona reference intended to execute this instruction.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"filtering",
			}).
			Validation("string ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("AGI-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentInstructionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentInstructionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentInstructionBuilder) GetOntology() string {
	return "agent_instruction"
}

func init() {
	builders.RegisterBuilder(NewAgentInstructionBuilder())
}
