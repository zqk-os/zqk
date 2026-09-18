package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// McpSpecBuilder builds the mcp_spec spec at version v2_0_0
// File: bldr_v2/mcp_spec_builder.go - version is encoded in package/directory name
type McpSpecBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMcpSpecBuilder creates a new builder for mcp_spec spec version v2_0_0
func NewMcpSpecBuilder() *McpSpecBuilder {
	builder := &McpSpecBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("mcp_spec", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Declarative MCP server configuration stored as a first-class kernel object.\\nThe spec payload configures prompts, resources, tools, tool groups, discovery,\\nand schema handlers. Lifecycle: mcp_spec_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addMcpSpecFields()

	return builder
}

// addMcpSpecFields adds the mcp_spec fields
func (b *McpSpecBuilder) addMcpSpecFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator").
			AutomationHooks("used by MCP loaders to select one canonical configuration").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("MCP runtime registration").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Stable configuration name used by MCP loaders").
			Security("non-sensitive").
			SystemUsage([]any{
				"storage lookup",
				"runtime configuration selection",
			}).
			Validation("Lowercase identifier").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			DisplayLength(40).
			Pattern(`^[a-z][a-z0-9_]*$`).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("identifier").
		WithProfileCode("MCPSPEC-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator").
			AutomationHooks("reported when MCP selects a configuration source").
			Cardinality("one").
			Criticality("association").
			Default("required").
			Dependencies("configuration origination workflow").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Record where the authoritative configuration originated").
			Security("non-sensitive").
			SystemUsage([]any{
				"provenance diagnostics",
				"fallback decisions",
				"audit",
			}).
			Validation("Non-empty provenance label").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			DisplayLength(32).
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MCPSPEC-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("spec", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator").
			AutomationHooks("parsed and validated before MCP runtime registration").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("MCP specification schema").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Declarative MCP configuration payload").
			Security("may contain confidential paths and prompt templates").
			SystemUsage([]any{
				"prompt registration",
				"resource registration",
				"tool registration",
				"schema handler registration",
			}).
			Validation("Valid YAML accepted by MCPSpec.Validate").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MCPSPEC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("operator").
			AutomationHooks("emitted in source provenance and diagnostics").
			Cardinality("one").
			Criticality("association").
			Default("required").
			Dependencies("configuration release process").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Identify the revision of the stored MCP configuration").
			Security("non-sensitive").
			SystemUsage([]any{
				"diagnostics",
				"migration",
				"audit",
			}).
			Validation("Non-empty version string").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			DisplayLength(24).
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("identifier").
		WithProfileCode("MCPSPEC-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *McpSpecBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *McpSpecBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *McpSpecBuilder) GetOntology() string {
	return "mcp_spec"
}

func init() {
	builders.RegisterBuilder(NewMcpSpecBuilder())
}
