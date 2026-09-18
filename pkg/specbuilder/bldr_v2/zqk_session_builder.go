package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ZqkSessionBuilder builds the zqk_session spec at version v2_0_0
// File: bldr_v2/zqk_session_builder.go - version is encoded in package/directory name
type ZqkSessionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewZqkSessionBuilder creates a new builder for zqk_session spec version v2_0_0
func NewZqkSessionBuilder() *ZqkSessionBuilder {
	builder := &ZqkSessionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("zqk_session", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a CLI or interactive session (e.g. a zqk invocation or agent session). Tracks session state and optional metadata.\\nAgent seat-workers bind runtime identity (executor/provider/model/persona/seat) on a child session with session_type=agent_worker; mesh agent_id remains a routing mailbox.\\nLifecycle: zqk_session_lifecycle.yaml.\\nTRACK: REQ-COMMS-RUNTIME-SESSION-001 / BLI-1786955190100310000-edcc34dd\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addZqkSessionFields()

	return builder
}

// addZqkSessionFields adds the zqk_session fields
func (b *ZqkSessionBuilder) addZqkSessionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("account_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from CLI security context (current user)").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("account object").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Account that owns this session (e.g. ACC-1785920548450214012-68b850c0 or logged-in user)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session attribution",
				"per-account session limits",
			}).
			Validation("Must match account ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ZQK-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by CLI or session manager").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("session lifecycle").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Stable identifier for the session (e.g. ZQK-001)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
			}).
			Validation("Must match ZQK session ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^ZQK-\d+$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("ZQK-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system/user").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional human-readable label for the session").
			Security("non-sensitive").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ZQK-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("session_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by CLI root (cli) or seat-worker StartWorkerSession (agent_worker)").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("cli").
			Dependencies("session lifecycle").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Distinguishes root CLI sessions from per-process agent seat-worker runtime sessions").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"provenance",
			}).
			Validation("enum cli or agent_worker").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{"cli", "agent_worker"}).
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("state").
		WithProfileCode("ZQK-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("parent_session_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set when creating agent_worker child under an active CLI session").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("zqk_session").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Parent zqk_session id (CLI/root) for an agent_worker child session").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"provenance",
			}).
			Validation("Must match ZQK session ID pattern when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^ZQK-\d+$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ZQK-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from seat-worker --agent-id (logical mesh seat mailbox)").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("mesh peer seats").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Logical mesh seat id this worker session serves; not derived from provider/model").
			Security("non-sensitive").
			SystemUsage([]any{
				"mesh routing attribution",
				"provenance",
			}).
			Validation("Non-empty seat id when session_type=agent_worker").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("ZQK-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from seat-worker --persona-ref").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("persona object").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Kernel persona id (PER-*) used for feed authorship during this worker session").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance",
				"COMMS attribution",
			}).
			Validation("PER-* when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ZQK-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("executor_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by seat-worker (e.g. agentx)").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Executor family driving cognition for this session (not the mesh seat name)").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance",
			}).
			Validation("Short token when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ZQK-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from llm.DefaultConfig(ctx).Provider at worker start").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("LLM client config").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("LLM API provider serving requests for this worker session").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance",
			}).
			Validation("Provider name when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ZQK-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider_profile_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("optional link to provider_profile when CAP resolves one").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("provider_profile").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Optional provider_profile object id when topology is objectified").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance",
				"CAP routing",
			}).
			Validation("PPRV-* when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ZQK-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("model_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set from llm.DefaultConfig(ctx).ChatModel at worker start").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("LLM client config").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Chat model serving requests for this worker session").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance",
			}).
			Validation("Model identifier when set").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ZQK-011"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ZqkSessionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ZqkSessionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ZqkSessionBuilder) GetOntology() string {
	return "zqk_session"
}

func init() {
	builders.RegisterBuilder(NewZqkSessionBuilder())
}
