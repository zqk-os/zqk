package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ProviderProfileBuilder builds the provider_profile spec at version v2_0_0
// File: bldr_v2/provider_profile_builder.go - version is encoded in package/directory name
type ProviderProfileBuilder struct {
	*builders.BaseSpecBuilder
}

// NewProviderProfileBuilder creates a new builder for provider_profile spec version v2_0_0
func NewProviderProfileBuilder() *ProviderProfileBuilder {
	builder := &ProviderProfileBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("provider_profile", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines the topology, operational constraints, and initialization sequence for LLM providers.\\nThis allows the CAP orchestrator to inject context window limits, disable SWA (Sliding Window Attention) \\nfor local llama.cpp endpoints, and tailor the prompt structure dynamically without hardcoding.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("constrainable")

	// Add fields
	builder.addProviderProfileFields()

	return builder
}

// addProviderProfileFields adds the provider_profile fields
func (b *ProviderProfileBuilder) addProviderProfileFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("base_url", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by the swarm engine to route requests.").
			Cardinality("one").
			Criticality("metadata").
			Default("http://127.0.0.1:8080/v1").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The URL prefix for the LLM API.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a valid URL string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("capabilities", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by the swarm engine to determine feature support.").
			Cardinality("many").
			Criticality("metadata").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of supported features (e.g. code_generation, validation, routing, json_mode).").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Array of capability strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("PPRV-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("concurrency_limit", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Limits parallel calls to this provider to prevent host starvation.").
			Cardinality("one").
			Criticality("metadata").
			Default("2").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Max concurrent execution threads allowed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context_window_limit", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Informs the graph bloat compactor and memory limits.").
			Cardinality("one").
			Criticality("metadata").
			Default(32768).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Max tokens or chars allowed in the context window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("endpoint_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Defines which adapter or connector to use.").
			Cardinality("one").
			Criticality("metadata").
			Default("openai_compatible").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The type of the API endpoint.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-001"))
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
			Purpose("Stable identifier.").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
			}).
			Validation("Must start with PPRV-").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("PPRV-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("initialization_sequence", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Sequence of actions (e.g. compress_cli_tools, verify_health).").
			Cardinality("many").
			Criticality("metadata").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Step-by-step startup process for the provider.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Array of step strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("PPRV-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("injection_rules", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Rules injected at different lifecycle stages.").
			Cardinality("many").
			Criticality("metadata").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Provider-specific constraints or instructions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Array of rule objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("list").
		WithProfileCode("PPRV-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_tool_schema_bytes", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by the tool compression logic.").
			Cardinality("one").
			Criticality("metadata").
			Default(15000).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Hard limit for JSON schema byte size to prevent SWA blowouts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("model_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Passed to the API for inference.").
			Cardinality("one").
			Criticality("metadata").
			Default("").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The specific model identifier (e.g. qwen3.6.Q4_K_M.gguf).").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
			}).
			Validation("Must be a string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PPRV-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("model_tier", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("Used by the CAP router to assign task difficulty levels.").
			Cardinality("one").
			Criticality("metadata").
			Default("tier_2_simple").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Operational tier (e.g. tier_1_complex, tier_2_simple, tier_3_light).").
			Security("non-sensitive").
			SystemUsage([]any{
				"provisioning",
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
		WithProfileCode("PPRV-008"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ProviderProfileBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ProviderProfileBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ProviderProfileBuilder) GetOntology() string {
	return "provider_profile"
}

func init() {
	builders.RegisterBuilder(NewProviderProfileBuilder())
}
