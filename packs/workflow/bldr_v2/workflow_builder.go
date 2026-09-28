package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// WorkflowBuilder builds the workflow spec at version v2_0_0
// File: bldr_v2/workflow_builder.go - version is encoded in package/directory name
type WorkflowBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWorkflowBuilder creates a new builder for workflow spec version v2_0_0
func NewWorkflowBuilder() *WorkflowBuilder {
	builder := &WorkflowBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("workflow", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines workflows that orchestrate object lifecycles, policies, templates, and other workflow-supporting items. Workflows can be scoped to specific roles or accounts, enabling role-based workflow presentation. A workflow represents a complete process or methodology (e.g., \\\"Feature Development Workflow\\\", \\\"Bug Fix Workflow\\\").\\nLifecycle: workflow_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addWorkflowFields()

	return builder
}

// addWorkflowFields adds the workflow fields
func (b *WorkflowBuilder) addWorkflowFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applicable_accounts", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("filters workflows by account for presentation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("account system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of account IDs that can use this workflow. Empty list means available to all accounts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"account-based filtering",
				"workflow presentation",
				"access control",
			}).
			Validation("list of account references (account:username or ACC-###).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("WFL-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("applicable_roles", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("filters workflows by role for presentation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("role system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of role IDs that can use this workflow. Empty list means available to all roles.").
			Security("non-sensitive").
			SystemUsage([]any{
				"role-based filtering",
				"workflow presentation",
				"access control",
			}).
			Validation("list of role references (role:ID or RLE-###).").
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
		WithProfileCode("WFL-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("blocking_check_config_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("links workflow to blocking check configuration for validation behavior.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("blocking check config.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to a blocking check configuration that applies to this workflow (overrides default).").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation behavior",
				"blocking check customization",
			}).
			Validation("reference to blocking check config (path or identifier).").
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
		WithProfileCode("WFL-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("used for workflow filtering and organization.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("workflow registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workflow category for organization (development, operations, planning, review, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"organization",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"development",
				"operations",
				"planning",
				"review",
				"testing",
				"deployment",
				"maintenance",
				"onboarding",
				"documentation",
				"mcp",
				"other",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WFL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("constraints", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("defines role-based constraints enforced during priority plan activation and workstream operations.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("workflow validation system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Defines role-based constraints for object creation and manipulation. Structure: {role_constraints: {role_id: {allowed_operations: [create, update, delete], allowed_kinds: [kind1, kind2], blocked_kinds: [kind3]}}, object_constraints: {kind: {required_roles: [role_id], blocked_roles: [role_id]}}}.").
			Security("non-sensitive").
			SystemUsage([]any{
				"constraint enforcement",
				"role-based access control",
				"workflow validation",
			}).
			Validation("object with structure: {role_constraints: object, object_constraints: object}.").
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
		WithProfileCode("WFL-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("used for workflow discovery and documentation.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("workflow registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Detailed description of the workflow, its purpose, and when to use it.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"workflow discovery",
				"guidance",
			}).
			Validation("markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WFL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("determines if workflow is available for use.").
			Cardinality("one").
			Criticality("composition").
			Default(true).
			Dependencies("workflow registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether this workflow is currently enabled and available for use.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"workflow availability",
			}).
			Validation("boolean.").
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
		WithProfileCode("WFL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("lifecycle_kinds", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("links workflows to lifecycle definitions for validation and state management.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("lifecycle definitions.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to lifecycle definitions that this workflow uses (e.g., backlog_item lifecycle, requirement lifecycle).").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle integration",
				"state management",
				"validation",
			}).
			Validation("list of object kind strings (e.g., [\\\\\\\"backlog_item\\\\\\\", \\\\\\\"requirement\\\\\\\", \\\\\\\"milestone\\\\\\\"]).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("WFL-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mcp_config", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("defines MCP-specific tool exposure rules for MCP workflows (category: mcp).").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("MCP server, workflow system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("MCP-specific configuration for workflows with category 'mcp'. Controls which CLI commands/tools are exposed via MCP server. Structure: {exposed_tools: [list of command patterns], blocked_tools: [list of command patterns], tool_constraints: {tool_name: {required_roles: [roles], blocked_roles: [roles]}}}. Only applies when category is 'mcp'.").
			Security("non-sensitive").
			SystemUsage([]any{
				"MCP tool registration",
				"MCP command filtering",
				"MCP access control",
			}).
			Validation("object with structure: {exposed_tools: list, blocked_tools: list, tool_constraints: object}.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("WFL-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("stores workflow-specific metadata.").
			Cardinality("one").
			Criticality("composition").
			Default(map[string]any{}).
			Dependencies("workflow system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Additional metadata for workflow configuration (e.g., automation triggers, integration settings).").
			Security("may contain sensitive configuration.").
			SystemUsage([]any{
				"workflow configuration",
				"integration settings",
			}).
			Validation("object (key-value pairs).").
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
		WithProfileCode("WFL-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("links workflows to policies for enforcement and guidance.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to policy objects that apply to this workflow.").
			Security("non-sensitive").
			SystemUsage([]any{
				"policy enforcement",
				"guidance",
				"compliance",
			}).
			Validation("list of policy references (POL-###).").
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
		WithProfileCode("WFL-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stages", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("defines workflow stages for orchestration and progress tracking.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("workflow orchestration.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Ordered list of workflow stages, each defining what happens at that stage. Each stage object contains: name, description, order, lifecycle_states (map of object kind to expected status), required_objects (list of object kinds that should exist), optional_objects (list of object kinds that may exist), policies (list of policy refs that apply), templates (list of template refs to use).").
			Security("non-sensitive").
			SystemUsage([]any{
				"workflow orchestration",
				"progress tracking",
				"guidance",
			}).
			Validation("list of stage objects with structure: {name: string, description: text, order: number, lifecycle_states: object, required_objects: list, optional_objects: list, policies: list, templates: list}.").
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
		WithProfileCode("WFL-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("template_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("links workflows to templates for artifact generation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("template system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to template objects used in this workflow.").
			Security("non-sensitive").
			SystemUsage([]any{
				"artifact generation",
				"automation",
			}).
			Validation("list of template references (TPL-###).").
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
		WithProfileCode("WFL-009"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *WorkflowBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *WorkflowBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *WorkflowBuilder) GetOntology() string {
	return "workflow"
}

func init() {
	builders.RegisterBuilder(NewWorkflowBuilder())
}
