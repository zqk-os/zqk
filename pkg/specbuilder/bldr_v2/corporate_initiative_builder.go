package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CorporateInitiativeBuilder builds the corporate_initiative spec at version v2_0_0
// File: bldr_v2/corporate_initiative_builder.go - version is encoded in package/directory name
type CorporateInitiativeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCorporateInitiativeBuilder creates a new builder for corporate_initiative spec version v2_0_0
func NewCorporateInitiativeBuilder() *CorporateInitiativeBuilder {
	builder := &CorporateInitiativeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("corporate_initiative", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Corporate initiative objects aggregate multiple Workstream OS projects/repositories into portfolios, business units, and corporate initiatives. Provides rollup metrics (PCS, EDD, D&B), cross-project dependency tracking, and portfolio-level visibility. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addCorporateInitiativeFields()

	return builder
}

// addCorporateInitiativeFields adds the corporate_initiative fields
func (b *CorporateInitiativeBuilder) addCorporateInitiativeFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_db", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("updated during portfolio aggregation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("aggregation engine, D&B detection.").
			Lifecycle("mutable (recalculated during aggregation).").
			Observability("yes").
			Purpose("Aggregated Dependencies & Blockers result across all projects.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"dashboards",
				"risk management",
			}).
			Validation("Must match DBResult structure.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CI-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_edd", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("updated during portfolio aggregation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("aggregation engine, EDD calculation.").
			Lifecycle("mutable (recalculated during aggregation).").
			Observability("yes").
			Purpose("Aggregated Effort Distribution Discrepancy across all projects.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"dashboards",
				"estimation accuracy",
			}).
			Validation("Percentage variance (can be negative for under-estimation).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("CI-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_pcs", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("updated during portfolio aggregation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("aggregation engine, PCS calculation.").
			Lifecycle("mutable (recalculated during aggregation).").
			Observability("yes").
			Purpose("Aggregated Project Confidence Score (0-100) across all projects in this initiative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"dashboards",
				"prioritization",
			}).
			Validation("Number between 0 and 100.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("CI-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregation_status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("triggers alerts if failed.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("aggregation engine.").
			Lifecycle("mutable (set during aggregation).").
			Observability("yes").
			Purpose("Status of last aggregation attempt (success/partial/failed).").
			Security("non-sensitive").
			SystemUsage([]any{
				"error tracking",
				"reporting",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"success",
				"partial",
				"failed",
			}).
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CI-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("cross_project_dependencies", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("updated during dependency analysis.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("dependency resolver.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Cross-project dependency relationships (e.g., \\\\\\\"project:A depends on project:B\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"dependency visualization",
				"risk management",
			}).
			Validation("Must reference existing projects and describe dependency.").
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
		WithProfileCode("CI-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_aggregated_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used to determine if aggregation is stale.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("aggregation engine.").
			Lifecycle("mutable (set during aggregation).").
			Observability("yes").
			Purpose("ISO-8601 timestamp when metrics were last aggregated.").
			Security("non-sensitive").
			SystemUsage([]any{
				"freshness tracking",
				"reporting",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CI-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("owner_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin.").
			AutomationHooks("ensures ownership is validated.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("authorization checks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to account/role representing the initiative owner.").
			Security("may contain PII—treat as confidential.").
			SystemUsage([]any{
				"permissions",
				"reporting",
			}).
			Validation("URI or structured reference pointing to an existing account/role.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("CI-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("project_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin.").
			AutomationHooks("triggers aggregation when updated.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("aggregation engine.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to Workstream OS instances/projects that are part of this initiative.").
			Security("non-sensitive").
			SystemUsage([]any{
				"aggregation",
				"reporting",
				"navigation",
			}).
			Validation("Must reference existing projects using project ID format or storage path.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("CI-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resource_allocation", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin.").
			AutomationHooks("used in resource optimization.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("resource tracker.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Resource allocation across projects (team assignments, budget, etc.).").
			Security("may contain sensitive resource information.").
			SystemUsage([]any{
				"resource planning",
				"reporting",
			}).
			Validation("Flexible structure (project -> resource mapping).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CI-008"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CorporateInitiativeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CorporateInitiativeBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CorporateInitiativeBuilder) GetOntology() string {
	return "corporate_initiative"
}

func init() {
	builders.RegisterBuilder(NewCorporateInitiativeBuilder())
}
