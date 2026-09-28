package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AgentArchitectureBuilder builds the agent_architecture spec at version v2_0_0
// File: bldr_v2/agent_architecture_builder.go - version is encoded in package/directory name
type AgentArchitectureBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentArchitectureBuilder creates a new builder for agent_architecture spec version v2_0_0
func NewAgentArchitectureBuilder() *AgentArchitectureBuilder {
	builder := &AgentArchitectureBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_architecture", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines the architecture for a specialized agent, including components, integration points, and system requirements. Documents all \\\"tendrils\\\" (connections and impacts) of an agent.\\nLifecycle: agent_architecture_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAgentArchitectureFields()

	return builder
}

// addAgentArchitectureFields adds the agent_architecture fields
func (b *AgentArchitectureBuilder) addAgentArchitectureFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("determines architecture requirements").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of agent this architecture defines.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_management",
				"architecture_documentation",
			}).
			Validation("enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"observer",
				"test_agent",
				"coder_agent",
				"devops_agent",
				"documentation_agent",
				"optimization_agent",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AGENT-ARCH-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("components", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for component tracking").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of components in this agent's architecture.").
			Security("non-sensitive").
			SystemUsage([]any{
				"architecture_documentation",
				"implementation_planning",
			}).
			Validation("List of component objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("AGENT-ARCH-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per kind sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier used across the graph (\\\\\\\"AGENT-ARCH-####\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^AGENT-ARCH-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^AGENT-ARCH-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("integration_points", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for integration tracking").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of integration points (tendrils) for this agent.").
			Security("non-sensitive").
			SystemUsage([]any{
				"architecture_documentation",
				"integration_planning",
			}).
			Validation("List of integration point objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("AGENT-ARCH-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("role_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for role linking").
			Cardinality("one").
			Criticality("association").
			Default("optional").
			Dependencies("role must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the role definition for this agent.").
			Security("non-sensitive").
			SystemUsage([]any{
				"role_management",
				"permission_tracking",
			}).
			Validation("Must reference existing role").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AGENT-ARCH-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentArchitectureBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentArchitectureBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentArchitectureBuilder) GetOntology() string {
	return "agent_architecture"
}

func init() {
	builders.RegisterBuilder(NewAgentArchitectureBuilder())
}
