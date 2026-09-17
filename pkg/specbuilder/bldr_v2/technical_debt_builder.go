package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TechnicalDebtBuilder builds the technical_debt spec at version v2_0_0
// File: bldr_v2/technical_debt_builder.go - version is encoded in package/directory name
type TechnicalDebtBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTechnicalDebtBuilder creates a new builder for technical_debt spec version v2_0_0
func NewTechnicalDebtBuilder() *TechnicalDebtBuilder {
	builder := &TechnicalDebtBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("technical_debt", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_unit").
		SetDescription("Technical debt items represent system issues that are not directly related to product features but support improving overall quality, streamlining delivery, and implementing self-improvement routines. These include code quality issues, infrastructure improvements, tooling enhancements, and process optimizations.\\nLifecycle: technical_debt_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("effort_aware")

	// Add fields
	builder.addTechnicalDebtFields()

	return builder
}

// addTechnicalDebtFields adds the technical_debt fields
func (b *TechnicalDebtBuilder) addTechnicalDebtFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("backlog_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for traceability, reporting").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("backlog item registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to backlog item tracking the resolution work (BLI-#### format)").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"reporting",
				"workflow integration",
			}).
			Validation("Must match backlog item ID pattern (BLI-####)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^BLI-.*$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("TDE-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("complexity_score", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (linter)").
			AutomationHooks("populated by linter for complexity-type debt").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("linter analysis").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Cyclomatic complexity score (for complexity-type debt)").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"reporting",
				"trend analysis",
			}).
			Validation("Positive integer").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("TDE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("debt_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for filtering, reporting, prioritization").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of technical debt (complexity, performance, maintainability, infrastructure, tooling, process, documentation, testing, security, observability)").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"prioritization",
				"reporting",
			}).
			Validation("Must be one of defined debt types").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"complexity",
				"performance",
				"maintainability",
				"infrastructure",
				"tooling",
				"process",
				"documentation",
				"testing",
				"security",
				"observability",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "searchable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("TDE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for documentation, reporting").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Detailed description of the technical debt issue").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"reporting",
				"communication",
			}).
			Validation("Free-form text, max 2000 chars").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(2000).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("TDE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("file_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for file-level tracking, automated fixes").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("file system").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("File path where the technical debt exists (e.g., \\\\\\\"cmd/zqk/object/list.go\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"automated fixes",
				"reporting",
			}).
			Validation("Valid file path relative to repository root").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(500).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("identifier").
		WithProfileCode("TDE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("function_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for function-level tracking").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("code analysis").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Function name where the technical debt exists (e.g., \\\\\\\"runList\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Valid function identifier").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(200).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("identifier").
		WithProfileCode("TDE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("impact_assessment", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/reviewer").
			AutomationHooks("used for prioritization").
			Cardinality("one").
			Criticality("composition").
			Default("low").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Impact assessment of the technical debt (low, medium, high, critical)").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"reporting",
				"resource allocation",
			}).
			Validation("Must be one of defined impact levels").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"low",
				"medium",
				"high",
				"critical",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TDE-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("linter_rule", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (linter)").
			AutomationHooks("populated by linter analysis").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("linter system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Linter rule that identified this technical debt (e.g., \\\\\\\"gocyclo\\\\\\\", \\\\\\\"gocritic:hugeParam\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
				"automated fixes",
			}).
			Validation("Valid linter rule identifier").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(100).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("TDE-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mitigation_plan", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used for documentation, reporting").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Plan for mitigating or resolving the technical debt").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"reporting",
				"planning",
			}).
			Validation("Free-form text, max 2000 chars").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(2000).
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("TDE-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("policy_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator").
			AutomationHooks("used for policy compliance tracking").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("policy registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to policy that identifies this as technical debt (POL-#### format)").
			Security("non-sensitive").
			SystemUsage([]any{
				"policy compliance",
				"reporting",
				"traceability",
			}).
			Validation("Must match policy ID pattern (POL-####)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^POL-[A-Z]+-\d{3}$`).
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("TDE-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resolution_notes", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("resolver").
			AutomationHooks("used for documentation, reporting").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Notes about how the technical debt was resolved").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"reporting",
				"learning",
			}).
			Validation("Free-form text, max 2000 chars").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(2000).
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("TDE-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tags", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner").
			AutomationHooks("used for filtering, grouping, discovery").
			Cardinality("zero_or_many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional tags for categorizing technical debt (e.g., \\\\\\\"continuous_improvement\\\\\\\", \\\\\\\"refactoring\\\\\\\", \\\\\\\"performance\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"organization",
				"discovery",
			}).
			Validation("Array of strings, each max 50 chars").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("TDE-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_resolution_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creator/owner").
			AutomationHooks("used for reminders, prioritization").
			Cardinality("one").
			Criticality("association").
			Default("calculated from created_at + 30 days").
			Dependencies("lifecycle system").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target date for resolving the technical debt (ISO 8601 format)").
			Security("non-sensitive").
			SystemUsage([]any{
				"reminders",
				"prioritization",
				"reporting",
				"SLA tracking",
			}).
			Validation("ISO 8601 date format (YYYY-MM-DD)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}$`).
			Required(true).
			Build()).
		WithTraits("filterable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("TDE-007"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TechnicalDebtBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TechnicalDebtBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TechnicalDebtBuilder) GetOntology() string {
	return "technical_debt"
}

func init() {
	builders.RegisterBuilder(NewTechnicalDebtBuilder())
}
