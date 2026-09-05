package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// McpSessionBuilder builds the mcp_session spec at version v2_0_0
// File: bldr_v2/mcp_session_builder.go - version is encoded in package/directory name
type McpSessionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMcpSessionBuilder creates a new builder for mcp_session spec version v2_0_0
func NewMcpSessionBuilder() *McpSessionBuilder {
	builder := &McpSessionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("mcp_session", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents an MCP client session and its authentication state. Tracks authentication status, account, roles, and permissions for MCP connections.\\nLifecycle: mcp_session_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addMcpSessionFields()

	return builder
}

// addMcpSessionFields adds the mcp_session fields
func (b *McpSessionBuilder) addMcpSessionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("account_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user/system").
			AutomationHooks("set during authentication").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("account object").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Account ID once authenticated (e.g., \\\\\\\"account:cursor-vscode\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication",
				"authorization",
			}).
			Validation("Must match account ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(account:[a-z0-9._-]+|ACC-\d{3,})$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("MCP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("authentication_status", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("updated by authentication flow").
			Cardinality("one").
			Criticality("composition").
			Default("pending_authentication").
			Dependencies("authentication flow").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Current authentication status (pending_authentication, authenticated, expired, revoked)").
			Security("non-sensitive").
			SystemUsage([]any{
				"authentication flow",
				"session management",
			}).
			Validation("Must be one of: pending_authentication, authenticated, expired, revoked").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"pending_authentication",
				"authenticated",
				"expired",
				"revoked",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("MCP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("client_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by MCP server on connection").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("MCP client connection").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Unique identifier for the MCP client connection").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"authentication",
			}).
			Validation("Client-provided identifier").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("MCP-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("client_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by MCP server from clientInfo").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("MCP client connection").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable name of the MCP client (e.g., \\\\\\\"cursor-vscode\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"logging",
			}).
			Validation("Client-provided name").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("MCP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set by MCP server on connection").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("MCP session lifecycle").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Stable identifier for an MCP session (supports client sessions and agent/test sessions)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"authentication",
			}).
			Validation("Must match MCP session ID pattern").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^MCP-\d+$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("MCP-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_activity", "datetime").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("updated on each MCP request").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("MCP server activity tracking").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 timestamp of last MCP activity").
			Security("non-sensitive").
			SystemUsage([]any{
				"session management",
				"timeout detection",
			}).
			Validation("ISO-8601 datetime").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("MCP-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("permissions", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user/system").
			AutomationHooks("set during authentication").
			Cardinality("zero_or_many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("role objects").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Permissions granted to this session after authentication").
			Security("non-sensitive").
			SystemUsage([]any{
				"authorization",
				"access control",
			}).
			Validation("Array of permission strings (format: \\\\\\\"operation:resource\\\\\\\")").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MCP-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("roles", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user/system").
			AutomationHooks("set during authentication").
			Cardinality("zero_or_many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("role objects").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Roles assigned to this session after authentication").
			Security("non-sensitive").
			SystemUsage([]any{
				"authorization",
				"access control",
			}).
			Validation("Array of role identifiers").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("MCP-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("set on session create (in_progress); set to disconnected on client disconnect").
			Cardinality("one").
			Criticality("composition").
			Default("in_progress").
			Dependencies("MCP session lifecycle").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Session lifecycle stage (in_progress while connected; disconnected on EOF/timeout/shutdown; eligible for retention when not in_progress)").
			Security("non-sensitive").
			SystemUsage([]any{
				"session tracking",
				"retention_tolerance (prunes disconnected/archived)",
			}).
			Validation("Must be one of in_progress, disconnected, archived, error (see mcp_session lifecycle)").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"in_progress",
				"disconnected",
				"archived",
				"error",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("MCP-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system").
			AutomationHooks("optional; may be set for agent/test sessions").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional human-readable label for the session").
			Security("non-sensitive").
			SystemUsage([]any{
				"observability",
				"debugging",
			}).
			Validation("Optional").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MCP-008"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *McpSessionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *McpSessionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *McpSessionBuilder) GetOntology() string {
	return "mcp_session"
}

func init() {
	builders.RegisterBuilder(NewMcpSessionBuilder())
}
