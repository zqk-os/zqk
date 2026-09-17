package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AgentOnboardingPreparationBuilder builds the agent_onboarding_preparation spec at version v2_0_0
// File: bldr_v2/agent_onboarding_preparation_builder.go - version is encoded in package/directory name
type AgentOnboardingPreparationBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentOnboardingPreparationBuilder creates a new builder for agent_onboarding_preparation spec version v2_0_0
func NewAgentOnboardingPreparationBuilder() *AgentOnboardingPreparationBuilder {
	builder := &AgentOnboardingPreparationBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_onboarding_preparation", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Tracks preparation work for onboarding specialized agents, including readiness criteria, infrastructure requirements, and preparation tasks. Ensures agents can onboard efficiently.\\nLifecycle: agent_onboarding_preparation_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAgentOnboardingPreparationFields()

	return builder
}

// addAgentOnboardingPreparationFields adds the agent_onboarding_preparation fields
func (b *AgentOnboardingPreparationBuilder) addAgentOnboardingPreparationFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("determines preparation requirements").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of agent being prepared for onboarding.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_management",
				"onboarding_tracking",
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
		WithTraits("filterable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AGENT-PREP-001"))
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
			Purpose("Stable identifier used across the graph (\\\\\\\"AGENT-PREP-####\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^AGENT-PREP-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^AGENT-PREP-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("preparation_status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for status tracking").
			Cardinality("one").
			Criticality("composition").
			Default("not_started").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Current status of preparation work.").
			Security("non-sensitive").
			SystemUsage([]any{
				"onboarding_tracking",
				"status_reporting",
			}).
			Validation("enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"not_started",
				"in_progress",
				"ready",
				"complete",
				"blocked",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGENT-PREP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("preparation_tasks", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for task tracking").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog items must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Backlog items that represent preparation tasks.").
			Security("non-sensitive").
			SystemUsage([]any{
				"task_tracking",
				"onboarding_management",
			}).
			Validation("Must reference existing backlog items").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AGENT-PREP-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("readiness_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for readiness validation").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of criteria that must be met before agent can onboard.").
			Security("non-sensitive").
			SystemUsage([]any{
				"readiness_validation",
			}).
			Validation("List of criterion objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("AGENT-PREP-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_date", "date").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for onboarding scheduling").
			Cardinality("one").
			Criticality("association").
			Default("optional").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target date for agent onboarding.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"scheduling",
			}).
			Validation("Valid date format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGENT-PREP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_workstream_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for workstream-agent linking").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("workstream must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstream where this agent will be activated.").
			Security("non-sensitive").
			SystemUsage([]any{
				"workstream_management",
				"onboarding_tracking",
			}).
			Validation("Must reference existing workstream").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AGENT-PREP-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentOnboardingPreparationBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentOnboardingPreparationBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentOnboardingPreparationBuilder) GetOntology() string {
	return "agent_onboarding_preparation"
}

func init() {
	builders.RegisterBuilder(NewAgentOnboardingPreparationBuilder())
}
