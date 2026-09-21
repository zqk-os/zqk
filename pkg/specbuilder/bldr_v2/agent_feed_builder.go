package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AgentFeedBuilder builds the agent_feed spec at version v2_0_0
// File: bldr_v2/agent_feed_builder.go - version is encoded in package/directory name
type AgentFeedBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAgentFeedBuilder creates a new builder for agent_feed spec version v2_0_0
func NewAgentFeedBuilder() *AgentFeedBuilder {
	builder := &AgentFeedBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("agent_feed", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Binds a logical **agent chat / IDE feed** to on-disk policy and event streams (lite file under `.zqk/agent-runtime/`\\nand append-only JSONL). Used by stewards and future Cursor/IDE integrations so delivery rules are process objects,\\nnot only local JSON. Runtime paths default through `pkg/datacell` (`AgentChatChannelConfigPath`,\\n`AgentChatChannelEventsJSONLPath`); optional overrides below apply when set.\\n**Structural (CAS):** identity, title, path overrides, contract version, notes—changes that redefine the binding.\\n**Runtime_delta (overlay):** high-churn toggles—`enabled`, `delivery_mode`—so operators can flip delivery without rewriting the structural blob.\\nLifecycle: agent_feed_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addAgentFeedFields()

	return builder
}

// addAgentFeedFields adds the agent_feed fields
func (b *AgentFeedBuilder) addAgentFeedFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("config_path_override", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("optional override for lite policy JSON path.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("path cache.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When set, steward uses this path instead of the default datacell alias for config.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("project-relative or absolute path.").
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
		WithProfileCode("AGF-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("contract_schema_version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("version JSONL event rows.").
			Cardinality("one").
			Criticality("composition").
			Default("1").
			Dependencies("JSONL writers.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Schema version string for events appended for this feed (e.g. `1`).").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("non-empty string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(32).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGF-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("delivery_mode", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("selects how much automation runs (notify vs clipboard vs paste).").
			Cardinality("one").
			Criticality("composition").
			Default("off").
			Dependencies("IDE hooks, notifications.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Coarse delivery mode for this feed binding.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"off",
				"log",
				"clipboard",
				"paste",
				"notify",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGF-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "bool").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("gates whether steward writes events or triggers IDE delivery.").
			Cardinality("one").
			Criticality("composition").
			Default(false).
			Dependencies("datacell paths.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When false, producers should not emit to the feed or paste into IDE.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGF-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("events_jsonl_path_override", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("optional override for JSONL event log path.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("path cache.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When set, writers append here instead of the default datacell events path.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("project-relative or absolute path.").
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
		WithProfileCode("AGF-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("note", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("none.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Free-form operator notes for this feed binding.").
			Security("may contain sensitive details.").
			SystemUsage([]any{
				"agent_feed",
			}).
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
		WithProfileCode("AGF-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("probe_command_substrings", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("optional postToolUse hook filter; at least one substring must appear in tool payload when non-empty.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("Cursor hook.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("e.g. zqk, bin/zqk — narrows probe to CLI-shaped invocations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGF-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("probe_tool_allowlist", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator.").
			AutomationHooks("optional postToolUse hook filter; materialized to agent_chat_channel.json for .cursor/hooks/post-tooluse-probe.sh.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("Cursor hook, IDE.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When non-empty, only these tool_name values (e.g. Shell, run_terminal_cmd) may append probe JSONL; case-insensitive AND with probe_command_substrings when both set.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_feed",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AGF-007"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AgentFeedBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AgentFeedBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AgentFeedBuilder) GetOntology() string {
	return "agent_feed"
}

func init() {
	builders.RegisterBuilder(NewAgentFeedBuilder())
}
