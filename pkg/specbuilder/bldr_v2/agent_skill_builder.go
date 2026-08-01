package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AgentSkillBuilder builds the agent_skill spec at version v2_0_0
// File: bldr_v2/agent_skill_builder.go - version is encoded in package/directory name
type AgentSkillBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentSkillBuilder creates a new builder for agent_skill spec version v2_0_0
func NewAgentSkillBuilder() *AgentSkillBuilder {
	builder := &AgentSkillBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_skill", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a distributable, codified agent skill or extension (e.g., Gemini CLI skill, Cursor rule, MCP tool). Integrating skills as system objects ensures procedural knowledge is versioned, shared, and propagates project culture and guidelines.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAgentSkillFields()

	return builder
}

// addAgentSkillFields adds the agent_skill fields
func (b *AgentSkillBuilder) addAgentSkillFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("instructions", "string").
		WithTraits("readable", "writable", "searchable").
		WithSemanticType("description"))
	b.AddFieldBuilder(builders.NewFieldBuilder("instructions_summary", "string").
		WithTraits("readable", "writable", "searchable").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider", "string").
		WithTraits("readable", "writable", "filterable").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentSkillBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentSkillBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentSkillBuilder) GetOntology() string {
	return "agent_skill"
}

func init() {
	builders.RegisterBuilder(NewAgentSkillBuilder())
}
