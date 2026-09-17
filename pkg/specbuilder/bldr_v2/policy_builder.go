package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// PolicyBuilder builds the policy spec at version v2_0_0
// File: bldr_v2/policy_builder.go - version is encoded in package/directory name
type PolicyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPolicyBuilder creates a new builder for policy spec version v2_0_0
func NewPolicyBuilder() *PolicyBuilder {
	builder := &PolicyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("policy", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Project-wide policies that establish standards, expectations, and best practices. Policies are required at project initialization and evolve throughout the delivery lifecycle. They provide a central index for AI agents to understand expectations before implementing.\\nLifecycle: policy_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPolicyFields()

	return builder
}

// addPolicyFields adds the policy fields
func (b *PolicyBuilder) addPolicyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("applicability", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("determines when policy applies.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{
				"exclusions":    []any{},
				"file_patterns": []any{},
				"object_types":  []any{},
				"workstreams":   []any{},
			}).
			Dependencies("enforcement hooks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scope of policy applicability (workstreams, object types, file patterns, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"enforcement",
			}).
			Validation("Structured object with optional fields like \\\\\\\"workstreams\\\\\\\", \\\\\\\"object_types\\\\\\\", \\\\\\\"file_patterns\\\\\\\", \\\\\\\"exclusions\\\\\\\".").
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
		WithProfileCode("POL-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("body", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used to generate guidance and enforcement hints.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("policy index, enforcement hooks.").
			Lifecycle("mutable (with version history).").
			Observability("yes").
			Purpose("The actual policy content, standards, and expectations.").
			Security("may contain sensitive workflows.").
			SystemUsage([]any{
				"documentation",
				"enforcement",
				"guidance",
			}).
			Validation("markdown allowed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for policy discovery and indexing.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("policy index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Policy category (architecture, code_quality, documentation, testing, security, workflow, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"indexing",
				"filtering",
				"organization",
			}).
			Validation("controlled vocab (architecture, code_quality, documentation, testing, security, workflow, git, ci_cd, etc.).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("effective_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("tracks when policy was established or updated.").
			Cardinality("one").
			Criticality("association").
			Default("creation date").
			Dependencies("policy index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When this policy version became effective.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal tracking",
				"compliance",
			}).
			Validation("ISO-8601 date.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("enforcement", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("configures how policy is enforced.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{
				"automated":        false,
				"reminder_enabled": true,
				"review_required":  false,
				"severity":         "medium",
			}).
			Dependencies("enforcement hooks, reminder system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Enforcement mechanism configuration (automated checks, reminders, review requirements).").
			Security("non-sensitive").
			SystemUsage([]any{
				"enforcement",
				"automation",
			}).
			Validation("Structured object with fields like \\\\\\\"automated\\\\\\\", \\\\\\\"reminder_enabled\\\\\\\", \\\\\\\"review_required\\\\\\\", \\\\\\\"severity\\\\\\\".").
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
		WithProfileCode("POL-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("examples", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to provide concrete examples to AI agents.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Code examples, patterns, or anti-patterns illustrating the policy.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"guidance",
			}).
			Validation("markdown allowed in list items.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for goal-policy traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this policy supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing goal IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("POL-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per category sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier for policy objects (POL-CATEGORY-### format).").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^POL-[A-Z]+-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^POL-[A-Z]+-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("POL-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone-policy traceability.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones this policy relates to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
			}).
			Validation("Must reference existing milestone IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("POL-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("determines enforcement strictness and reminder priority.").
			Cardinality("one").
			Criticality("composition").
			Default("guideline").
			Dependencies("enforcement hooks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of policy (standard=mandatory, requirement=must follow, guideline=should follow, best_practice=recommended, anti_pattern=what to avoid).").
			Security("non-sensitive").
			SystemUsage([]any{
				"enforcement",
				"prioritization",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"standard",
				"requirement",
				"guideline",
				"best_practice",
				"anti_pattern",
			}).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_patterns", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for cross-referencing and discovery.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy index, architecture patterns.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to architecture patterns, ADRs, or other policies related to this policy.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"discovery",
			}).
			Validation("Must reference existing doc_entry, decision, or policy IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("POL-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("review_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("triggers policy review reminders.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler, reminder system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Next scheduled review date for this policy.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
				"reminders",
			}).
			Validation("ISO-8601 date.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("validation_overlays", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/architect.").
			AutomationHooks("overlays these DSL definitions onto target objects during verification.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("verification engine.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Universal Verification DSL definitions that are automatically injected as validation overlays into objects governed by this policy.").
			Security("non-sensitive").
			SystemUsage([]any{
				"verification",
				"enforcement",
			}).
			Validation("List of VerificationStrategy objects.").
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
		WithProfileCode("POL-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("tracks policy changes over time.").
			Cardinality("one").
			Criticality("association").
			Default(objects.InitialFieldVersion).
			Dependencies("policy index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Policy version (SemVer) for tracking policy evolution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"versioning",
				"evolution tracking",
			}).
			Validation("SemVer pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d+\.\d+\.\d+$`).
			Required(false).
			Build()).
		WithTraits("filterable", "modifiable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("POL-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for workstream-specific enforcement.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this policy applies to (if specific).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"enforcement",
			}).
			Validation("Must reference existing workstream IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("POL-012"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PolicyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PolicyBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PolicyBuilder) GetOntology() string {
	return "policy"
}

func init() {
	builders.RegisterBuilder(NewPolicyBuilder())
}
